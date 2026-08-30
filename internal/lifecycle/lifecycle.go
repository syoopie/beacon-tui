// Package lifecycle performs the mutating server operations: start, stop,
// force-kill, and the operator's "mark stopped" for a server stuck in Unknown.
// Every operation runs under the host op lock for its whole duration, including
// the stop timeout wait, so a second start cannot slip in beside a stop.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/syoopie/beacon-tui/internal/config"
	"github.com/syoopie/beacon-tui/internal/oplock"
	"github.com/syoopie/beacon-tui/internal/reconcile"
	"github.com/syoopie/beacon-tui/internal/server"
	"github.com/syoopie/beacon-tui/internal/supervisor"
)

// Manager owns mutation for one beacon process.
type Manager struct {
	sup         supervisor.Supervisor
	dirs        config.Dirs
	stopTimeout time.Duration
	poll        time.Duration
	now         func() time.Time
	busy        sync.Mutex
}

// DefaultPoll is how often Stop rechecks whether the session is gone.
const DefaultPoll = time.Second

func NewManager(sup supervisor.Supervisor, dirs config.Dirs, stopTimeout time.Duration) *Manager {
	if stopTimeout <= 0 {
		stopTimeout = 60 * time.Second
	}
	return &Manager{
		sup:         sup,
		dirs:        dirs,
		stopTimeout: stopTimeout,
		poll:        DefaultPoll,
		now:         time.Now,
	}
}

// ErrBusy is returned when this process is already running a mutating operation.
var ErrBusy = errors.New("an operation is already running in this beacon")

// StopOutcome reports how a Stop ended.
type StopOutcome struct {
	Stopped  bool // the session is gone
	TimedOut bool // the timeout elapsed with the session still up; force-kill is now the operator's call
	Waited   time.Duration
}

// hold takes the in-process guard and then the host lock. The returned release
// undoes both and is safe to call once via defer.
func (m *Manager) hold(dir string, op oplock.OpKind) (func(), error) {
	if !m.busy.TryLock() {
		return nil, ErrBusy
	}
	h, err := oplock.Acquire(dir, op)
	if err != nil {
		m.busy.Unlock()
		return nil, err
	}
	return func() {
		_ = h.Release()
		m.busy.Unlock()
	}, nil
}

func (m *Manager) lockDir() string { return m.dirs.State }

// Start brings a server up. It is a no-op when the session already exists, an
// error when the server is in Unknown, when its start script does not exec, or
// when its port is taken.
func (m *Manager) Start(ctx context.Context, spec server.Spec, all []server.Spec) (server.Spec, error) {
	release, err := m.hold(m.lockDir(), oplock.OpStart)
	if err != nil {
		return spec, err
	}
	defer release()

	exists, err := m.sup.Exists(ctx, spec.Session)
	if err != nil {
		return spec, err
	}
	if exists {
		return spec, nil
	}
	switch spec.State.LastKnown {
	case server.StatusStarting, server.StatusRunning, server.StatusStopping:
		return spec, fmt.Errorf("%s is in an unknown state (session gone, last seen up); mark it stopped or inspect the host before starting", spec.ID)
	}
	if !spec.Exec.Launchable() {
		return spec, fmt.Errorf("%s: start script does not exec its command; re-run import to patch it", spec.ID)
	}
	if block := reconcile.CheckPort(spec.Port, spec.ID, all); block.Blocked() {
		return spec, fmt.Errorf("%s: cannot start on port %d: %s", spec.ID, spec.Port, block)
	}

	launch := supervisor.Launch{
		Session: spec.Session,
		Dir:     spec.Dir,
		Command: spec.Start,
		LogFile: spec.LogFile,
	}
	if err := m.sup.Start(ctx, launch); err != nil {
		return spec, fmt.Errorf("%s: launching: %w", spec.ID, err)
	}

	return m.writeState(spec, server.StatusRunning)
}

// Stop asks a server to shut down and waits, under the lock, until its session
// is gone or the timeout elapses. It never force-kills on its own.
func (m *Manager) Stop(ctx context.Context, spec server.Spec) (server.Spec, StopOutcome, error) {
	release, err := m.hold(m.lockDir(), oplock.OpStop)
	if err != nil {
		return spec, StopOutcome{}, err
	}
	defer release()

	exists, err := m.sup.Exists(ctx, spec.Session)
	if err != nil {
		return spec, StopOutcome{}, err
	}
	if !exists {
		spec, err = m.writeState(spec, server.StatusStopped)
		return spec, StopOutcome{Stopped: true}, err
	}

	// Record Stopping before we send the command: a crash between here and the
	// session's exit must reconcile to Unknown, not to a Stopped we cannot prove.
	if spec, err = m.writeState(spec, server.StatusStopping); err != nil {
		return spec, StopOutcome{}, err
	}
	if err := m.sup.SendKeys(ctx, spec.Session, "stop"); err != nil {
		return spec, StopOutcome{}, fmt.Errorf("%s: sending stop: %w", spec.ID, err)
	}

	start := m.now()
	deadline := start.Add(m.stopTimeout)
	ticker := time.NewTicker(m.poll)
	defer ticker.Stop()
	for {
		gone, err := m.sessionGone(ctx, spec.Session)
		if err != nil {
			return spec, StopOutcome{}, err
		}
		if gone {
			spec, err = m.writeState(spec, server.StatusStopped)
			return spec, StopOutcome{Stopped: true, Waited: m.now().Sub(start)}, err
		}
		if !m.now().Before(deadline) {
			return spec, StopOutcome{TimedOut: true, Waited: m.now().Sub(start)}, nil
		}
		select {
		case <-ctx.Done():
			return spec, StopOutcome{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

// ForceKill tears down the tmux session. It is the operator's move after Stop
// reports a timeout, never automatic.
func (m *Manager) ForceKill(ctx context.Context, spec server.Spec) (server.Spec, error) {
	release, err := m.hold(m.lockDir(), oplock.OpForceKill)
	if err != nil {
		return spec, err
	}
	defer release()

	if err := m.sup.Kill(ctx, spec.Session); err != nil {
		return spec, fmt.Errorf("%s: killing session: %w", spec.ID, err)
	}
	return m.writeState(spec, server.StatusStopped)
}

// MarkStopped records Stopped for a server the operator has confirmed is down
// after it reconciled to Unknown. This is the only way out of Unknown.
func (m *Manager) MarkStopped(spec server.Spec) (server.Spec, error) {
	release, err := m.hold(m.lockDir(), oplock.OpWriteConfig)
	if err != nil {
		return spec, err
	}
	defer release()
	return m.writeState(spec, server.StatusStopped)
}

func (m *Manager) sessionGone(ctx context.Context, s server.Session) (bool, error) {
	exists, err := m.sup.Exists(ctx, s)
	return !exists, err
}

func (m *Manager) writeState(spec server.Spec, status server.Status) (server.Spec, error) {
	spec.State.LastKnown = status
	spec.State.UpdatedAt = m.now()
	if err := config.SaveSpec(m.dirs, spec); err != nil {
		return spec, fmt.Errorf("%s: recording %s: %w", spec.ID, status, err)
	}
	return spec, nil
}
