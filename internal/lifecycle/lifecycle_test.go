package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sunyupei/beacon-tui/internal/config"
	"github.com/sunyupei/beacon-tui/internal/server"
	"github.com/sunyupei/beacon-tui/internal/supervisor"
)

type fakeSup struct {
	mu         sync.Mutex
	exists     bool
	startErr   error
	sentKeys   []string
	onStop     func()
	startCount int
}

func (f *fakeSup) Start(_ context.Context, _ supervisor.Launch) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCount++
	if f.startErr != nil {
		return f.startErr
	}
	f.exists = true
	return nil
}

func (f *fakeSup) Exists(context.Context, server.Session) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.exists, nil
}

func (f *fakeSup) SendKeys(_ context.Context, _ server.Session, line string) error {
	f.mu.Lock()
	f.sentKeys = append(f.sentKeys, line)
	stop := line == "stop" && f.onStop != nil
	f.mu.Unlock()
	if stop {
		f.onStop()
	}
	return nil
}

func (f *fakeSup) Kill(context.Context, server.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exists = false
	return nil
}

func (f *fakeSup) setExists(v bool) {
	f.mu.Lock()
	f.exists = v
	f.mu.Unlock()
}

func testDirs(t *testing.T) config.Dirs {
	t.Helper()
	root := t.TempDir()
	return config.Dirs{Config: root + "/config", State: root + "/state"}
}

func testSpec(t *testing.T, dirs config.Dirs, exec server.ExecState, last server.Status) server.Spec {
	t.Helper()
	id, err := server.ParseID("survival")
	if err != nil {
		t.Fatal(err)
	}
	return server.Spec{
		ID:      id,
		Dir:     "/srv/survival",
		Start:   "./run.sh",
		Script:  "run.sh",
		Port:    25565,
		Session: server.SessionFor(id),
		LogFile: dirs.LogFile(id),
		Exec:    exec,
		State:   server.State{LastKnown: last},
	}
}

func newManager(sup supervisor.Supervisor, dirs config.Dirs) *Manager {
	m := NewManager(sup, dirs, time.Second)
	m.poll = time.Millisecond
	return m
}

func TestStartIsNoOpWhenSessionExists(t *testing.T) {
	dirs := testDirs(t)
	sup := &fakeSup{exists: true}
	m := newManager(sup, dirs)

	spec := testSpec(t, dirs, server.ExecOK, server.StatusStopped)
	if _, err := m.Start(context.Background(), spec, nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if sup.startCount != 0 {
		t.Fatalf("Start launched a process though the session existed (%d calls)", sup.startCount)
	}
}

func TestStartRefusedFromUnknown(t *testing.T) {
	dirs := testDirs(t)
	sup := &fakeSup{exists: false}
	m := newManager(sup, dirs)

	spec := testSpec(t, dirs, server.ExecOK, server.StatusRunning) // session gone, last seen up => Unknown
	_, err := m.Start(context.Background(), spec, nil)
	if err == nil || sup.startCount != 0 {
		t.Fatalf("Start from Unknown: err=%v startCount=%d, want refusal", err, sup.startCount)
	}
}

func TestStartRefusedWhenScriptDoesNotExec(t *testing.T) {
	dirs := testDirs(t)
	sup := &fakeSup{}
	m := newManager(sup, dirs)

	spec := testSpec(t, dirs, server.ExecMissing, server.StatusStopped)
	if _, err := m.Start(context.Background(), spec, nil); err == nil {
		t.Fatal("Start accepted a spec whose script does not exec")
	}
}

func TestStartRefusedOnPortClaimedByAnotherSpec(t *testing.T) {
	dirs := testDirs(t)
	sup := &fakeSup{}
	m := newManager(sup, dirs)

	spec := testSpec(t, dirs, server.ExecOK, server.StatusStopped)
	rival := spec
	rival.ID = "creative"
	_, err := m.Start(context.Background(), spec, []server.Spec{spec, rival})
	if err == nil || sup.startCount != 0 {
		t.Fatalf("Start with a port rival: err=%v, want refusal", err)
	}
}

func TestStartRecordsRunning(t *testing.T) {
	dirs := testDirs(t)
	sup := &fakeSup{}
	m := newManager(sup, dirs)

	spec := testSpec(t, dirs, server.ExecOK, server.StatusStopped)
	got, err := m.Start(context.Background(), spec, []server.Spec{spec})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got.State.LastKnown != server.StatusRunning {
		t.Fatalf("recorded state = %v, want running", got.State.LastKnown)
	}
	reloaded, err := config.LoadSpec(dirs.ServerFile(spec.ID))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.State.LastKnown != server.StatusRunning {
		t.Fatalf("persisted state = %v, want running", reloaded.State.LastKnown)
	}
}

func TestStopSendsStopAndReportsGone(t *testing.T) {
	dirs := testDirs(t)
	sup := &fakeSup{exists: true}
	sup.onStop = func() { sup.setExists(false) }
	m := newManager(sup, dirs)

	spec := testSpec(t, dirs, server.ExecOK, server.StatusRunning)
	got, outcome, err := m.Stop(context.Background(), spec)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !outcome.Stopped || outcome.TimedOut {
		t.Fatalf("outcome = %+v, want Stopped", outcome)
	}
	if len(sup.sentKeys) != 1 || sup.sentKeys[0] != "stop" {
		t.Fatalf("sent keys = %v, want [stop]", sup.sentKeys)
	}
	if got.State.LastKnown != server.StatusStopped {
		t.Fatalf("state = %v, want stopped", got.State.LastKnown)
	}
}

func TestStopTimesOutWithoutKilling(t *testing.T) {
	dirs := testDirs(t)
	sup := &fakeSup{exists: true} // never goes away
	m := newManager(sup, dirs)

	m.stopTimeout = 50 * time.Millisecond
	m.now = advancingClock(time.Now(), 60*time.Millisecond)

	spec := testSpec(t, dirs, server.ExecOK, server.StatusRunning)
	got, outcome, err := m.Stop(context.Background(), spec)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !outcome.TimedOut || outcome.Stopped {
		t.Fatalf("outcome = %+v, want TimedOut", outcome)
	}
	if got.State.LastKnown != server.StatusStopping {
		t.Fatalf("state = %v, want stopping (beacon must not downgrade on timeout)", got.State.LastKnown)
	}
	// Session was left alone.
	if exists, _ := sup.Exists(context.Background(), spec.Session); !exists {
		t.Fatal("Stop killed the session on timeout")
	}
}

func TestForceKillClearsSessionAndRecordsStopped(t *testing.T) {
	dirs := testDirs(t)
	sup := &fakeSup{exists: true}
	m := newManager(sup, dirs)

	spec := testSpec(t, dirs, server.ExecOK, server.StatusStopping)
	got, err := m.ForceKill(context.Background(), spec)
	if err != nil {
		t.Fatalf("ForceKill: %v", err)
	}
	if exists, _ := sup.Exists(context.Background(), spec.Session); exists {
		t.Fatal("session still exists after ForceKill")
	}
	if got.State.LastKnown != server.StatusStopped {
		t.Fatalf("state = %v, want stopped", got.State.LastKnown)
	}
}

func TestSecondOperationInSameProcessIsBusy(t *testing.T) {
	dirs := testDirs(t)
	sup := &fakeSup{exists: true}
	sup.onStop = func() {} // stop never completes on its own
	m := newManager(sup, dirs)
	m.stopTimeout = time.Hour

	spec := testSpec(t, dirs, server.ExecOK, server.StatusRunning)

	started := make(chan struct{})
	go func() {
		close(started)
		m.Stop(context.Background(), spec)
	}()
	<-started
	time.Sleep(20 * time.Millisecond)

	_, err := m.Start(context.Background(), spec, []server.Spec{spec})
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("concurrent Start error = %v, want ErrBusy", err)
	}
}

// advancingClock returns now for the first read and now+ahead for every read
// after, which is enough to drive one poll and then trip the deadline.
func advancingClock(now time.Time, ahead time.Duration) func() time.Time {
	var calls int
	var mu sync.Mutex
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls <= 2 {
			return now
		}
		return now.Add(ahead)
	}
}
