// Package procstat reads a running process's memory, CPU and elapsed run time
// from ps. It is how beacon shows the weight of a server's JVM without linking a
// platform library: ps is on every macOS and Linux box that has tmux.
package procstat

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Stat is one sample of a process.
type Stat struct {
	At         time.Time     // when the sample was taken
	RSS        int64         // resident set size, bytes
	MemPercent float64       // RSS as a share of the host's physical memory
	CPUPercent float64       // CPU use, 100 per busy core; see Rate
	CPUTime    time.Duration // CPU time consumed since the process started
	Uptime     time.Duration // how long the process has been running
	MaxHeap    int64         // the JVM's -Xmx in bytes, 0 when the args do not set one
}

// Sample runs `ps` for one PID. dir is the process's working directory, against
// which a JVM @argfile is resolved. A pid that is gone, or any ps failure, is an
// error: the caller shows "unavailable" rather than a stale number.
//
// CPUPercent is what ps reports, which on Linux is the average over the whole
// run. Rate gives the current figure from two samples.
func Sample(ctx context.Context, pid int, dir string) (Stat, error) {
	if pid <= 0 {
		return Stat{}, fmt.Errorf("procstat: invalid pid %d", pid)
	}
	at := time.Now()
	out, err := exec.CommandContext(ctx, "ps", "-ww", "-o", "rss=,%cpu=,%mem=,time=,etime=,args=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return Stat{}, fmt.Errorf("procstat: ps for pid %d: %w", pid, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) < 5 {
		return Stat{}, fmt.Errorf("procstat: pid %d not reported by ps", pid)
	}
	rssKiB, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return Stat{}, fmt.Errorf("procstat: rss %q: %w", fields[0], err)
	}
	cpu, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return Stat{}, fmt.Errorf("procstat: cpu %q: %w", fields[1], err)
	}
	mem, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return Stat{}, fmt.Errorf("procstat: mem %q: %w", fields[2], err)
	}
	cpuTime, err := parseETime(fields[3])
	if err != nil {
		return Stat{}, err
	}
	uptime, err := parseETime(fields[4])
	if err != nil {
		return Stat{}, err
	}
	return Stat{
		At:         at,
		RSS:        rssKiB * 1024,
		MemPercent: mem,
		CPUPercent: cpu,
		CPUTime:    cpuTime,
		Uptime:     uptime,
		MaxHeap:    maxHeap(fields[5:], dir),
	}, nil
}

// Rate is the CPU use between two samples of the same process, 100 per busy
// core. ok is false when the samples cannot be compared: out of order, or from
// a process that restarted in between.
func Rate(prev, cur Stat) (pct float64, ok bool) {
	wall := cur.At.Sub(prev.At)
	busy := cur.CPUTime - prev.CPUTime
	if wall <= 0 || busy < 0 || cur.Uptime < prev.Uptime {
		return 0, false
	}
	return float64(busy) / float64(wall) * 100, true
}

// maxHeap finds the -Xmx a JVM was launched with, expanding @argfiles the way
// the java launcher does. The last setting wins, as it does for the JVM.
func maxHeap(args []string, dir string) int64 {
	var heap int64
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "-Xmx"):
			if n, ok := parseSize(strings.TrimPrefix(a, "-Xmx")); ok {
				heap = n
			}
		case strings.HasPrefix(a, "-XX:MaxHeapSize="):
			if n, ok := parseSize(strings.TrimPrefix(a, "-XX:MaxHeapSize=")); ok {
				heap = n
			}
		case strings.HasPrefix(a, "@") && len(a) > 1:
			if n := maxHeap(readArgFile(a[1:], dir), dir); n > 0 {
				heap = n
			}
		}
	}
	return heap
}

// readArgFile returns the whitespace-separated arguments of a java @argfile,
// skipping # comment lines. A file that cannot be read contributes nothing.
func readArgFile(path, dir string) []string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var args []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		args = append(args, strings.Fields(line)...)
	}
	return args
}

// parseSize reads a JVM memory size: a byte count with an optional k, m, g or t
// suffix in either case.
func parseSize(s string) (int64, bool) {
	mult := int64(1)
	if s != "" {
		switch s[len(s)-1] {
		case 'k', 'K':
			mult = 1 << 10
		case 'm', 'M':
			mult = 1 << 20
		case 'g', 'G':
			mult = 1 << 30
		case 't', 'T':
			mult = 1 << 40
		}
		if mult > 1 {
			s = s[:len(s)-1]
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n * mult, true
}

// parseETime reads the duration columns ps prints: etime as [[DD-]hh:]mm:ss,
// and time, which is the same shape on Linux and mm:ss.ss on macOS.
func parseETime(s string) (time.Duration, error) {
	orig := s
	days := 0
	if i := strings.IndexByte(s, '-'); i >= 0 {
		d, err := strconv.Atoi(s[:i])
		if err != nil {
			return 0, fmt.Errorf("procstat: duration days %q: %w", orig, err)
		}
		days, s = d, s[i+1:]
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("procstat: duration %q", orig)
	}
	var h, m int
	var err error
	if len(parts) == 3 {
		if h, err = strconv.Atoi(parts[0]); err != nil {
			return 0, fmt.Errorf("procstat: duration hours %q: %w", orig, err)
		}
		parts = parts[1:]
	}
	if m, err = strconv.Atoi(parts[0]); err != nil {
		return 0, fmt.Errorf("procstat: duration minutes %q: %w", orig, err)
	}
	sec, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, fmt.Errorf("procstat: duration seconds %q: %w", orig, err)
	}
	return time.Duration(days)*24*time.Hour +
		time.Duration(h)*time.Hour +
		time.Duration(m)*time.Minute +
		time.Duration(sec*float64(time.Second)), nil
}
