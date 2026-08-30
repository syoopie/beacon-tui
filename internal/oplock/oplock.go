// Package oplock is beacon's host-wide mutual exclusion for mutating operations.
// Any number of beacon processes may read config and tail logs at once; only one
// may start, stop, force-kill, import, patch a script, or write config at a time.
// The lock is a PID-bearing file under the state directory, with stale-holder
// recovery when the previous holder died without releasing it.
package oplock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
)

// OpKind names the mutating operation a lock is held for. It is recorded in the
// lockfile so a blocked process can tell the operator what is running.
type OpKind uint8

const (
	OpStart OpKind = iota
	OpStop
	OpForceKill
	OpImport
	OpPatchScript
	OpWriteConfig
)

var opNames = [...]string{
	OpStart:       "start",
	OpStop:        "stop",
	OpForceKill:   "force-kill",
	OpImport:      "import",
	OpPatchScript: "patch-script",
	OpWriteConfig: "write-config",
}

func (k OpKind) String() string {
	if int(k) >= len(opNames) {
		return fmt.Sprintf("op(%d)", uint8(k))
	}
	return opNames[k]
}

// Holder is what the current lockfile records about whoever holds it.
type Holder struct {
	PID   int       `toml:"pid"`
	Op    string    `toml:"op"`
	Since time.Time `toml:"since"`
}

// HeldError is returned by Acquire when a live process already holds the lock.
type HeldError struct{ Holder Holder }

func (e *HeldError) Error() string {
	return fmt.Sprintf("another beacon is doing %s (pid %d, since %s)",
		e.Holder.Op, e.Holder.PID, e.Holder.Since.Format(time.Kitchen))
}

// Handle is a held lock. Release it exactly once, when the operation finishes.
type Handle struct {
	path string
	done bool
}

const lockName = "beacon.lock"

// Acquire takes the host lock for op, creating the state directory if needed.
// It returns a *HeldError when a living process holds the lock, and reclaims the
// lock when the recorded holder is gone.
func Acquire(stateDir string, op OpKind) (*Handle, error) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("state directory: %w", err)
	}
	path := filepath.Join(stateDir, lockName)

	for attempt := 0; attempt < 3; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			body, encErr := toml.Marshal(Holder{PID: os.Getpid(), Op: op.String(), Since: time.Now()})
			if encErr == nil {
				_, encErr = f.Write(body)
			}
			if closeErr := f.Close(); encErr == nil {
				encErr = closeErr
			}
			if encErr != nil {
				os.Remove(path)
				return nil, fmt.Errorf("writing lock: %w", encErr)
			}
			return &Handle{path: path}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("opening lock: %w", err)
		}

		holder, readErr := readHolder(path)
		if readErr == nil && processAlive(holder.PID) {
			return nil, &HeldError{Holder: holder}
		}
		// The holder is dead or the file is unreadable. Drop the stale file and
		// try to create it again; a rival racing the same recovery will win the
		// O_EXCL create and we will see its live holder.
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("clearing stale lock: %w", err)
		}
	}
	return nil, errors.New("lock contended by repeated stale recovery; try again")
}

// Release removes the lockfile. Calling it more than once is harmless.
func (h *Handle) Release() error {
	if h == nil || h.done {
		return nil
	}
	h.done = true
	if err := os.Remove(h.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("releasing lock: %w", err)
	}
	return nil
}

func readHolder(path string) (Holder, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Holder{}, err
	}
	var h Holder
	if err := toml.Unmarshal(data, &h); err != nil {
		return Holder{}, err
	}
	if h.PID <= 0 {
		return Holder{}, fmt.Errorf("lock records no pid")
	}
	return h, nil
}

// processAlive reports whether pid names a live process. Signal 0 performs the
// permission and existence checks without delivering a signal.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
