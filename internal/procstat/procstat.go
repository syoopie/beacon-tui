// Package procstat reads a running process's memory, CPU and elapsed run time
// from ps. It is how beacon shows the weight of a server's JVM without linking a
// platform library: ps is on every macOS and Linux box that has tmux.
package procstat

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Stat is one sample of a process.
type Stat struct {
	RSS        int64         // resident set size, bytes
	CPUPercent float64       // recent CPU use, as ps reports it
	Uptime     time.Duration // how long the process has been running
}

// Sample runs `ps` for one PID. A pid that is gone, or any ps failure, is an
// error: the caller shows "unavailable" rather than a stale number.
func Sample(ctx context.Context, pid int) (Stat, error) {
	if pid <= 0 {
		return Stat{}, fmt.Errorf("procstat: invalid pid %d", pid)
	}
	out, err := exec.CommandContext(ctx, "ps", "-o", "rss=,%cpu=,etime=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return Stat{}, fmt.Errorf("procstat: ps for pid %d: %w", pid, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) < 3 {
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
	uptime, err := parseETime(fields[2])
	if err != nil {
		return Stat{}, err
	}
	return Stat{RSS: rssKiB * 1024, CPUPercent: cpu, Uptime: uptime}, nil
}

// parseETime reads the elapsed-time column ps prints as [[DD-]hh:]mm:ss.
func parseETime(s string) (time.Duration, error) {
	days := 0
	if i := strings.IndexByte(s, '-'); i >= 0 {
		d, err := strconv.Atoi(s[:i])
		if err != nil {
			return 0, fmt.Errorf("procstat: etime days %q: %w", s[:i], err)
		}
		days, s = d, s[i+1:]
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("procstat: etime %q", s)
	}
	var h, m, sec int
	var err error
	if len(parts) == 3 {
		if h, err = strconv.Atoi(parts[0]); err != nil {
			return 0, fmt.Errorf("procstat: etime hours %q: %w", parts[0], err)
		}
		parts = parts[1:]
	}
	if m, err = strconv.Atoi(parts[0]); err != nil {
		return 0, fmt.Errorf("procstat: etime minutes %q: %w", parts[0], err)
	}
	if sec, err = strconv.Atoi(parts[1]); err != nil {
		return 0, fmt.Errorf("procstat: etime seconds %q: %w", parts[1], err)
	}
	return time.Duration(days)*24*time.Hour +
		time.Duration(h)*time.Hour +
		time.Duration(m)*time.Minute +
		time.Duration(sec)*time.Second, nil
}
