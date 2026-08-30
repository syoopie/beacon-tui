package reconcile

import (
	"context"
	"errors"
	"testing"

	"github.com/syoopie/beacon-tui/internal/server"
	"github.com/syoopie/beacon-tui/internal/supervisor"
)

type fakeSup struct {
	present map[server.Session]bool
	err     error
}

func (f fakeSup) Start(context.Context, supervisor.Launch) error { return nil }
func (f fakeSup) Exists(_ context.Context, s server.Session) (bool, error) {
	return f.present[s], f.err
}
func (f fakeSup) SendKeys(context.Context, server.Session, string) error { return nil }
func (f fakeSup) Kill(context.Context, server.Session) error             { return nil }

func spec(t *testing.T, id string, last server.Status) server.Spec {
	t.Helper()
	pid, err := server.ParseID(id)
	if err != nil {
		t.Fatalf("ParseID(%q): %v", id, err)
	}
	return server.Spec{
		ID:      pid,
		Session: server.SessionFor(pid),
		State:   server.State{LastKnown: last},
	}
}

func TestDerive(t *testing.T) {
	cases := []struct {
		name    string
		exists  bool
		last    server.Status
		want    server.Status
		warning bool
	}{
		{"live session is running", true, server.StatusStopped, server.StatusRunning, false},
		{"gone after stopped is stopped", false, server.StatusStopped, server.StatusStopped, false},
		{"gone after running is unknown", false, server.StatusRunning, server.StatusUnknown, true},
		{"gone after starting is unknown", false, server.StatusStarting, server.StatusUnknown, true},
		{"gone after stopping is unknown", false, server.StatusStopping, server.StatusUnknown, true},
		{"gone after unknown stays stopped", false, server.StatusUnknown, server.StatusStopped, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, warning := derive(c.exists, c.last)
			if got != c.want {
				t.Errorf("derive(%v, %v) = %v, want %v", c.exists, c.last, got, c.want)
			}
			if (warning != "") != c.warning {
				t.Errorf("derive(%v, %v) warning = %q, want present=%v", c.exists, c.last, warning, c.warning)
			}
		})
	}
}

func TestRunReportsPerSpec(t *testing.T) {
	up := spec(t, "survival", server.StatusRunning)
	lost := spec(t, "creative", server.StatusRunning)
	sup := fakeSup{present: map[server.Session]bool{up.Session: true}}

	reports, err := Run(context.Background(), sup, []server.Spec{up, lost})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reports[0].Derived != server.StatusRunning {
		t.Errorf("survival derived = %v, want running", reports[0].Derived)
	}
	if reports[1].Derived != server.StatusUnknown || reports[1].Warning == "" {
		t.Errorf("creative report = %+v, want unknown with a warning", reports[1])
	}
}

func TestRunAbortsOnSupervisorError(t *testing.T) {
	boom := errors.New("tmux server not running")
	_, err := Run(context.Background(), fakeSup{err: boom}, []server.Spec{spec(t, "survival", server.StatusStopped)})
	if !errors.Is(err, boom) {
		t.Fatalf("Run error = %v, want it to wrap %v", err, boom)
	}
}
