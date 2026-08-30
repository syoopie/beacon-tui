// Package procstat reads a running process's memory and CPU use from ps. It is
// how beacon shows the weight of a server's JVM without linking a platform
// library: ps is on every macOS and Linux box that has tmux.
package procstat

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Stat is one sample of a process.
type Stat struct {
	RSS        int64   // resident set size, bytes
	CPUPercent float64 // recent CPU use, as ps reports it
}

// Sample runs `ps` for one PID. A pid that is gone, or any ps failure, is an
// error: the caller shows "unavailable" rather than a stale number.
func Sample(ctx context.Context, pid int) (Stat, error) {
	if pid <= 0 {
		return Stat{}, fmt.Errorf("procstat: invalid pid %d", pid)
	}
	out, err := exec.CommandContext(ctx, "ps", "-o", "rss=,%cpu=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return Stat{}, fmt.Errorf("procstat: ps for pid %d: %w", pid, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
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
	return Stat{RSS: rssKiB * 1024, CPUPercent: cpu}, nil
}
