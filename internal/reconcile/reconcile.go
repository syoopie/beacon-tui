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

// Report is one server's reconciled state for a single pass.
type Report struct {
	ID            server.ID
	SessionExists bool
	LastKnown     server.Status
	Derived       server.Status
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
		return server.StatusUnknown, "tmux session is gone but the server was last seen up; mark it stopped or inspect the host"
	default:
		return server.StatusStopped, ""
	}
}
