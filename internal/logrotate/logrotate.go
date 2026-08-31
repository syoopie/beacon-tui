// Package logrotate keeps a server's captured console log from growing without
// bound. The log is fed by a shell redirect inside the tmux pane (exec >>
// logfile), so the writing file descriptor stays open for the life of the JVM
// and the file cannot simply be renamed out of the way. Instead Beacon copies
// the live file to a timestamped gzip archive and truncates it in place. The
// redirect is O_APPEND, so the next write after the truncate lands at offset
// zero with no gap. A handful of lines written during the copy can be lost;
// for a game-server console that is an acceptable trade for a bounded log.
package logrotate

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Policy controls when a log is rotated and how much history is kept.
type Policy struct {
	MaxLiveBytes int64 // rotate once the live file grows past this
	BudgetBytes  int64 // prune oldest archives until their combined size is under this
}

// DefaultPolicy rotates at 10 MiB and keeps about 450 MB of gzip archives.
var DefaultPolicy = Policy{
	MaxLiveBytes: 10 << 20,
	BudgetBytes:  450 << 20,
}

const archiveTimeFormat = "20060102T150405Z"

// Due reports whether logPath has grown past the policy limit, without taking
// any lock. Callers use it to skip the locked Rotate call in the common case.
func Due(logPath string, p Policy) bool {
	info, err := os.Stat(logPath)
	return err == nil && info.Size() > p.MaxLiveBytes
}

// Rotate archives and truncates logPath if it has grown past the policy limit,
// then prunes old archives back under the size budget. It reports whether it
// rotated. A missing log file is not an error.
func Rotate(logPath string, p Policy) (bool, error) {
	info, err := os.Stat(logPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Size() <= p.MaxLiveBytes {
		return false, nil
	}

	archive := archiveName(logPath, time.Now().UTC())
	if err := archiveGzip(logPath, archive); err != nil {
		return false, err
	}
	if err := os.Truncate(logPath, 0); err != nil {
		return true, fmt.Errorf("truncate %s: %w", logPath, err)
	}
	if err := prune(logPath, p.BudgetBytes); err != nil {
		return true, err
	}
	return true, nil
}

// archiveName is "<dir>/<name>.<timestamp>.log.gz" for a live log at
// "<dir>/<name>.log".
func archiveName(logPath string, at time.Time) string {
	base := strings.TrimSuffix(filepath.Base(logPath), ".log")
	return filepath.Join(filepath.Dir(logPath), base+"."+at.Format(archiveTimeFormat)+".log.gz")
}

// archiveGzip writes a gzip copy of src to dst, via a temp file so a crash
// mid-copy cannot leave a half-written archive in place.
func archiveGzip(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".rotate-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once the rename succeeds

	zw := gzip.NewWriter(tmp)
	if _, err := io.Copy(zw, in); err != nil {
		_ = zw.Close()
		_ = tmp.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}

// prune deletes the oldest archives for a log until the combined size of what
// remains is under budget. Archive names sort chronologically, so the newest
// are kept.
func prune(logPath string, budget int64) error {
	base := strings.TrimSuffix(filepath.Base(logPath), ".log")
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(logPath), base+".*.log.gz"))
	if err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(matches))) // newest first

	var kept int64
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if kept+info.Size() <= budget {
			kept += info.Size()
			continue
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}
