//go:build unix

package lifecycle

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/syoopie/beacon-tui/internal/config"
	"github.com/syoopie/beacon-tui/internal/oplock"
	"github.com/syoopie/beacon-tui/internal/server"
	"github.com/syoopie/beacon-tui/internal/tmux"
)

const stopsOnStop = `#!/bin/sh
echo "server started"
while IFS= read -r line; do
  echo "console: $line"
  if [ "$line" = "stop" ]; then
    echo "shutting down"
    exit 0
  fi
done
`

const ignoresStop = `#!/bin/sh
echo "server started"
while IFS= read -r line; do
  echo "console: $line (ignored)"
done
`

func liveManager(t *testing.T, stopTimeout time.Duration) (*Manager, config.Dirs) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not on PATH")
	}
	root := t.TempDir()
	dirs := config.Dirs{Config: filepath.Join(root, "config"), State: filepath.Join(root, "state")}
	m := NewManager(&tmux.Client{}, dirs, stopTimeout)
	m.poll = 100 * time.Millisecond
	return m, dirs
}

func stubServer(t *testing.T, dirs config.Dirs, name, script string) server.Spec {
	t.Helper()
	id, err := server.ParseID(name + "-" + strconv.Itoa(os.Getpid()))
	if err != nil {
		t.Fatalf("ParseID: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return server.Spec{
		ID:      id,
		Dir:     dir,
		Start:   "./run.sh",
		Script:  "run.sh",
		Port:    41565,
		Session: server.SessionFor(id),
		LogFile: dirs.LogFile(id),
		Exec:    server.ExecOK,
		State:   server.State{LastKnown: server.StatusStopped},
	}
}

func killSessionOnCleanup(t *testing.T, s server.Session) {
	t.Cleanup(func() {
		exec.Command("tmux", "kill-session", "-t", "="+string(s)).Run()
	})
}

func waitSessionGone(t *testing.T, s server.Session) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if exec.Command("tmux", "has-session", "-t", "="+string(s)).Run() != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("session %s still alive", s)
}

func TestStartThenGracefulStop(t *testing.T) {
	m, dirs := liveManager(t, 5*time.Second)
	spec := stubServer(t, dirs, "graceful", stopsOnStop)
	killSessionOnCleanup(t, spec.Session)

	spec, err := m.Start(context.Background(), spec, []server.Spec{spec})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if spec.State.LastKnown != server.StatusRunning {
		t.Fatalf("state after Start = %v", spec.State.LastKnown)
	}

	spec, outcome, err := m.Stop(context.Background(), spec)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !outcome.Stopped || outcome.TimedOut {
		t.Fatalf("stop outcome = %+v, want graceful", outcome)
	}
	if spec.State.LastKnown != server.StatusStopped {
		t.Fatalf("state after Stop = %v", spec.State.LastKnown)
	}
	waitSessionGone(t, spec.Session)

	data, err := os.ReadFile(spec.LogFile)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "console: stop") {
		t.Fatalf("log did not record the stop command:\n%s", data)
	}
}

func TestStopTimeoutThenForceKill(t *testing.T) {
	m, dirs := liveManager(t, 1*time.Second)
	spec := stubServer(t, dirs, "stubborn", ignoresStop)
	killSessionOnCleanup(t, spec.Session)

	spec, err := m.Start(context.Background(), spec, []server.Spec{spec})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	spec, outcome, err := m.Stop(context.Background(), spec)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !outcome.TimedOut {
		t.Fatalf("stop outcome = %+v, want TimedOut", outcome)
	}
	if spec.State.LastKnown != server.StatusStopping {
		t.Fatalf("state after timeout = %v, want stopping", spec.State.LastKnown)
	}
	if exec.Command("tmux", "has-session", "-t", "="+string(spec.Session)).Run() != nil {
		t.Fatal("session died on its own; the stub was supposed to ignore stop")
	}

	spec, err = m.ForceKill(context.Background(), spec)
	if err != nil {
		t.Fatalf("ForceKill: %v", err)
	}
	if spec.State.LastKnown != server.StatusStopped {
		t.Fatalf("state after ForceKill = %v", spec.State.LastKnown)
	}
	waitSessionGone(t, spec.Session)
}

func TestStartRefusedWhileAnotherProcessHoldsTheLock(t *testing.T) {
	m, dirs := liveManager(t, time.Second)
	spec := stubServer(t, dirs, "contended", stopsOnStop)

	sleep := exec.Command("sleep", "30")
	if err := sleep.Start(); err != nil {
		t.Fatalf("spawn lock holder: %v", err)
	}
	t.Cleanup(func() { sleep.Process.Kill(); sleep.Wait() })

	if err := os.MkdirAll(dirs.State, 0o700); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	body, _ := toml.Marshal(oplock.Holder{PID: sleep.Process.Pid, Op: "stop", Since: time.Now()})
	if err := os.WriteFile(filepath.Join(dirs.State, "beacon.lock"), body, 0o600); err != nil {
		t.Fatalf("seed lock: %v", err)
	}

	_, err := m.Start(context.Background(), spec, []server.Spec{spec})
	var held *oplock.HeldError
	if !errors.As(err, &held) {
		t.Fatalf("Start error = %v, want *oplock.HeldError", err)
	}
}
