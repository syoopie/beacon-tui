// Package lifecycle performs the mutating server operations: start, stop,
// force-kill, and the operator's "mark stopped" for a server stuck in Unknown.
// Every operation runs under the host op lock for its whole duration, including
// the stop timeout wait, so a second start cannot slip in beside a stop.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/syoopie/beacon-tui/internal/config"
	"github.com/syoopie/beacon-tui/internal/importdetect"
	"github.com/syoopie/beacon-tui/internal/logrotate"
	"github.com/syoopie/beacon-tui/internal/mcprops"
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
var ErrBusy = errors.New("an operation is already running in this Beacon")

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
	if ok, err := mcprops.EULAAccepted(spec.Dir); err != nil {
		return spec, fmt.Errorf("%s: reading eula.txt: %w", spec.ID, err)
	} else if !ok {
		return spec, fmt.Errorf("%s: the Minecraft EULA has not been accepted; open the server's menu and choose \"Accept the Minecraft EULA\"", spec.ID)
	}
	if block := reconcile.CheckPort(spec.Port, spec.ID, all); block.Blocked() {
		return spec, fmt.Errorf("%s: cannot start on port %d: %s", spec.ID, spec.Port, block)
	}
	if spec.Java != "" {
		if err := runnableFile(spec.Java); err != nil {
			return spec, fmt.Errorf("%s: its Java setting points at %s, which %s; fix it in Launch settings", spec.ID, spec.Java, err)
		}
	}

	launch := supervisor.Launch{
		Session: spec.Session,
		Dir:     spec.Dir,
		Command: spec.Start,
		LogFile: spec.LogFile,
	}
	if spec.Java != "" {
		launch.JavaBinDir = filepath.Dir(spec.Java)
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

// SendConsole delivers one line to a running server's console. It runs under
// the host lock, like every other mutating op, so a console line cannot
// interleave with a start or a stop. It refuses when the session is not up:
// there is nothing to type at.
func (m *Manager) SendConsole(ctx context.Context, spec server.Spec, line string) error {
	release, err := m.hold(m.lockDir(), oplock.OpConsole)
	if err != nil {
		return err
	}
	defer release()

	exists, err := m.sup.Exists(ctx, spec.Session)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%s is not running; there is no console to send to", spec.ID)
	}
	if err := m.sup.SendKeys(ctx, spec.Session, line); err != nil {
		return fmt.Errorf("%s: sending console line: %w", spec.ID, err)
	}
	return nil
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

// EditProperties writes the given key=value pairs into the server's
// server.properties, then mirrors the port and the RCON block into the spec so
// the rest of Beacon sees the change without a re-import. It takes the host
// lock, like every write to a server's files.
func (m *Manager) EditProperties(spec server.Spec, edits map[string]string) (server.Spec, error) {
	release, err := m.hold(m.lockDir(), oplock.OpWriteConfig)
	if err != nil {
		return spec, err
	}
	defer release()

	props, err := mcprops.LoadProperties(spec.Dir)
	if err != nil {
		return spec, fmt.Errorf("%s: reading server.properties: %w", spec.ID, err)
	}
	for k, v := range edits {
		props.Set(k, v)
	}
	if err := props.Save(); err != nil {
		return spec, fmt.Errorf("%s: writing server.properties: %w", spec.ID, err)
	}

	spec.Port = props.Port()
	spec.RCON = props.RCON()
	if err := config.SaveSpec(m.dirs, spec); err != nil {
		return spec, fmt.Errorf("%s: saving spec: %w", spec.ID, err)
	}
	return spec, nil
}

// SaveSpec persists an edited spec under the host lock. The UI uses it for the
// launch-settings edit, which rewrites the spec file the same way a config edit
// does and so must serialize with every other mutating op.
func (m *Manager) SaveSpec(spec server.Spec) (server.Spec, error) {
	release, err := m.hold(m.lockDir(), oplock.OpWriteConfig)
	if err != nil {
		return spec, err
	}
	defer release()

	if err := config.SaveSpec(m.dirs, spec); err != nil {
		return spec, fmt.Errorf("%s: saving spec: %w", spec.ID, err)
	}
	return spec, nil
}

// DetectCommands fills a spec's [commands] mc_version and loader from a fresh
// scan of its directory when they are blank, and persists the result under the
// host lock. It backfills a server imported before detection existed the first
// time Beacon loads it, and never touches a value the operator set by hand.
// changed reports whether anything was written.
func (m *Manager) DetectCommands(spec server.Spec) (out server.Spec, changed bool, err error) {
	if spec.Commands.MCVersion != "" && spec.Commands.Loader != "" {
		return spec, false, nil
	}
	mcVersion, loader := importdetect.Identify(spec.Dir)
	if spec.Commands.MCVersion == "" && mcVersion != "" {
		spec.Commands.MCVersion = mcVersion
		changed = true
	}
	if spec.Commands.Loader == "" && loader != "" {
		spec.Commands.Loader = loader
		changed = true
	}
	if !changed {
		return spec, false, nil
	}

	release, err := m.hold(m.lockDir(), oplock.OpWriteConfig)
	if err != nil {
		return spec, false, err
	}
	defer release()

	if err := config.SaveSpec(m.dirs, spec); err != nil {
		return spec, false, fmt.Errorf("%s: saving detected command settings: %w", spec.ID, err)
	}
	return spec, true, nil
}

// ApplyScriptPatch runs the exec patch on a server's start script, re-inspects
// it, and records the new exec state in the spec, all under the host lock so it
// cannot race a start.
func (m *Manager) ApplyScriptPatch(spec server.Spec, patch importdetect.Patch) (server.Spec, error) {
	release, err := m.hold(m.lockDir(), oplock.OpPatchScript)
	if err != nil {
		return spec, err
	}
	defer release()

	if err := importdetect.Apply(patch); err != nil {
		return spec, fmt.Errorf("%s: patching the start script: %w", spec.ID, err)
	}
	check, err := importdetect.InspectScript(patch.Path)
	if err != nil {
		return spec, fmt.Errorf("%s: re-inspecting the start script: %w", spec.ID, err)
	}
	spec.Exec = check.State
	if err := config.SaveSpec(m.dirs, spec); err != nil {
		return spec, fmt.Errorf("%s: saving spec: %w", spec.ID, err)
	}
	return spec, nil
}

// AcceptEULA writes eula=true into the server's eula.txt under the host lock.
func (m *Manager) AcceptEULA(spec server.Spec) error {
	release, err := m.hold(m.lockDir(), oplock.OpWriteConfig)
	if err != nil {
		return err
	}
	defer release()

	if err := mcprops.AcceptEULA(spec.Dir); err != nil {
		return fmt.Errorf("%s: accepting the EULA: %w", spec.ID, err)
	}
	return nil
}

// RotateLog archives and truncates a server's captured console log if it has
// grown past the rotation policy. The size check is done without the lock; the
// lock is taken only when a rotation is actually due, so the common no-op case
// never contends with start or stop.
func (m *Manager) RotateLog(logPath string) (bool, error) {
	if !logrotate.Due(logPath, logrotate.DefaultPolicy) {
		return false, nil
	}
	release, err := m.hold(m.lockDir(), oplock.OpRotateLog)
	if err != nil {
		return false, err
	}
	defer release()
	return logrotate.Rotate(logPath, logrotate.DefaultPolicy)
}

func (m *Manager) sessionGone(ctx context.Context, s server.Session) (bool, error) {
	exists, err := m.sup.Exists(ctx, s)
	return !exists, err
}

// runnableFile reports why path is not an executable regular file, or nil when
// it is one. The error reads as a clause after "which".
func runnableFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.New("does not exist")
		}
		return errors.New("cannot be read")
	}
	if info.IsDir() {
		return errors.New("is a directory, not the java binary")
	}
	if info.Mode().Perm()&0o111 == 0 {
		return errors.New("is not executable")
	}
	return nil
}

func (m *Manager) writeState(spec server.Spec, status server.Status) (server.Spec, error) {
	spec.State.LastKnown = status
	spec.State.UpdatedAt = m.now()
	if err := config.SaveSpec(m.dirs, spec); err != nil {
		return spec, fmt.Errorf("%s: recording %s: %w", spec.ID, status, err)
	}
	return spec, nil
}
