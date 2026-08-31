// Package reconcile compares recorded server state against tmux reality. It is
// read-only: it never starts, stops, or writes anything. The lifecycle package
// owns mutation.
package reconcile

import (
	"context"
	"fmt"

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
		derived, warning := derive(exists, s.State.LastKnown)
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
func derive(sessionExists bool, lastKnown server.Status) (server.Status, string) {
	if sessionExists {
		return server.StatusRunning, ""
	}
	switch lastKnown {
	case server.StatusStarting, server.StatusRunning, server.StatusStopping:
		return server.StatusUnknown, "Beacon lost track of this server: its tmux session vanished while it was running. Check whether it is really down before starting it again."
	default:
		return server.StatusStopped, ""
	}
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
