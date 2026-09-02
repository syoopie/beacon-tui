// Package reconcile compares recorded server state against tmux reality. It is
// read-only: it never starts, stops, or writes anything. The lifecycle package
// owns mutation.
package reconcile

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/syoopie/beacon-tui/internal/server"
	"github.com/syoopie/beacon-tui/internal/supervisor"
)

// PortHealth is a live TCP probe of a server's listen port, a signal kept
// separate from Derived status. A running session whose port has not opened yet
// is still coming up; one whose port never opens is wedged. Status cannot tell
// those apart because both have a live tmux session.
type PortHealth int

const (
	PortUnprobed PortHealth = iota // no live session, or the spec carries no port
	PortOpen                       // a TCP connect to the port succeeded
	PortClosed                     // the session is up but nothing accepts on the port
)

// Report is one server's reconciled state for a single pass.
type Report struct {
	ID            server.ID
	SessionExists bool
	LastKnown     server.Status
	Derived       server.Status
	PortHealth    PortHealth
	// Warning is operator-facing text, non-empty only when reality and the
	// recorded state disagree in a way the operator must resolve.
	Warning string
}

// Run reconciles every spec against the supervisor. A supervisor error aborts
// the pass rather than reporting a guessed state for that server.
func Run(ctx context.Context, sup supervisor.Supervisor, specs []server.Spec) ([]Report, error) {
	reports := make([]Report, 0, len(specs))
	for _, s := range specs {
		exists, err := sup.Exists(ctx, s.Session)
		if err != nil {
			return nil, fmt.Errorf("reconcile %s: %w", s.ID, err)
		}
		derived := derive(exists, s.State.LastKnown)
		warning := ""
		if derived == server.StatusUnknown {
			warning = vanishedWarning(s.ID, s.LogFile)
		}
		reports = append(reports, Report{
			ID:            s.ID,
			SessionExists: exists,
			LastKnown:     s.State.LastKnown,
			Derived:       derived,
			PortHealth:    probePort(exists, s.Port),
			Warning:       warning,
		})
	}
	return reports, nil
}

// derive turns "is the session there" plus "what did we last write" into a
// status. A live session is Running. A missing session is Stopped unless we
// last believed the server was up, in which case it is Unknown: beacon will not
// silently downgrade a server it may have lost track of to Stopped, because that
// is how a second Start causes a port collision.
func derive(sessionExists bool, lastKnown server.Status) server.Status {
	if sessionExists {
		return server.StatusRunning
	}
	switch lastKnown {
	case server.StatusStarting, server.StatusRunning, server.StatusStopping:
		return server.StatusUnknown
	default:
		return server.StatusStopped
	}
}

// vanishedWarning is the operator-facing text for a server that reconciled to
// Unknown: its session is gone and beacon did not stop it. The last line of its
// captured log is the exit reason (a Java version gate, an out-of-memory kill, a
// stack trace) and is quoted verbatim so a failed start explains itself instead
// of reading as a mystery crash.
func vanishedWarning(id server.ID, logFile string) string {
	w := "Beacon did not stop " + string(id) + ", but its session is gone."
	if tail := lastLogLine(logFile); tail != "" {
		w += ` Its log ends: "` + tail + `"`
	}
	return w
}

// lastLogLine returns the final non-blank line of a captured console log,
// stripped of ANSI and control bytes and shortened to fit a notice. Only the
// tail of the file is read. Empty when the log cannot be read or holds nothing.
func lastLogLine(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return ""
	}
	const window = 8 << 10
	if info.Size() > window {
		if _, err := f.Seek(info.Size()-window, io.SeekStart); err != nil {
			return ""
		}
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(buf), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := tidyLine(lines[i]); s != "" {
			return ansi.Truncate(s, 200, "…")
		}
	}
	return ""
}

func tidyLine(line string) string {
	line = ansi.Strip(line)
	line = strings.TrimPrefix(strings.TrimLeft(line, "\r "), "> ")
	line = strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, line)
	return strings.TrimSpace(line)
}

// probePort dials the server's listen port when its session is live. A dead
// session or a spec with no port is left Unprobed rather than reported closed.
func probePort(sessionExists bool, port int) PortHealth {
	if !sessionExists || port <= 0 {
		return PortUnprobed
	}
	if osListening(port) {
		return PortOpen
	}
	return PortClosed
}
