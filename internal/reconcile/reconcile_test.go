package reconcile

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
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
func (f fakeSup) PID(context.Context, server.Session) (int, error)       { return 0, nil }

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
		name   string
		exists bool
		last   server.Status
		want   server.Status
	}{
		{"live session is running", true, server.StatusStopped, server.StatusRunning},
		{"gone after stopped is stopped", false, server.StatusStopped, server.StatusStopped},
		{"gone after running is unknown", false, server.StatusRunning, server.StatusUnknown},
		{"gone after starting is unknown", false, server.StatusStarting, server.StatusUnknown},
		{"gone after stopping is unknown", false, server.StatusStopping, server.StatusUnknown},
		{"gone after unknown stays stopped", false, server.StatusUnknown, server.StatusStopped},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := derive(c.exists, c.last); got != c.want {
				t.Errorf("derive(%v, %v) = %v, want %v", c.exists, c.last, got, c.want)
			}
		})
	}
}

func TestVanishedWarningQuotesTheLogTail(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "boom.log")
	body := "[12:00:00] [main/INFO]: loading\n" +
		"\x1b[31mMinecraft 26.1 and newer requires running the server with Java 25 or above.\x1b[0m\n\n"
	if err := os.WriteFile(log, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	gone := spec(t, "creative", server.StatusRunning)
	gone.LogFile = log
	reports, err := Run(context.Background(), fakeSup{}, []server.Spec{gone})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	w := reports[0].Warning
	if !strings.Contains(w, "requires running the server with Java 25 or above") {
		t.Errorf("warning does not carry the log tail: %q", w)
	}
	if strings.Contains(w, "\x1b") {
		t.Errorf("warning still has ANSI: %q", w)
	}
}

func TestVanishedWarningWithoutAReadableLog(t *testing.T) {
	gone := spec(t, "creative", server.StatusRunning)
	gone.LogFile = filepath.Join(t.TempDir(), "does-not-exist.log")
	reports, err := Run(context.Background(), fakeSup{}, []server.Spec{gone})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if w := reports[0].Warning; w == "" || strings.Contains(w, "Its log ends") {
		t.Errorf("want a bare warning with no log clause, got %q", w)
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

func TestRunProbesPortHealth(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	openPort := ln.Addr().(*net.TCPAddr).Port

	bound := spec(t, "survival", server.StatusRunning)
	bound.Port = openPort
	wedged := spec(t, "creative", server.StatusRunning)
	wedged.Port = openPort + 1 // nothing listens here
	down := spec(t, "skyblock", server.StatusStopped)
	down.Port = openPort

	sup := fakeSup{present: map[server.Session]bool{
		bound.Session:  true,
		wedged.Session: true,
	}}

	reports, err := Run(context.Background(), sup, []server.Spec{bound, wedged, down})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reports[0].PortHealth != PortOpen {
		t.Errorf("bound port health = %v, want PortOpen", reports[0].PortHealth)
	}
	if reports[1].PortHealth != PortClosed {
		t.Errorf("wedged port health = %v, want PortClosed", reports[1].PortHealth)
	}
	if reports[2].PortHealth != PortUnprobed {
		t.Errorf("stopped port health = %v, want PortUnprobed", reports[2].PortHealth)
	}
}

func TestRunAbortsOnSupervisorError(t *testing.T) {
	boom := errors.New("tmux server not running")
	_, err := Run(context.Background(), fakeSup{err: boom}, []server.Spec{spec(t, "survival", server.StatusStopped)})
	if !errors.Is(err, boom) {
		t.Fatalf("Run error = %v, want it to wrap %v", err, boom)
	}
}
