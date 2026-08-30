package oplock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
)

func TestAcquireThenReleaseThenReacquire(t *testing.T) {
	dir := t.TempDir()

	h, err := Acquire(dir, OpStart)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, lockName)); err != nil {
		t.Fatalf("lockfile not written: %v", err)
	}
	if err := h.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if err := h.Release(); err != nil {
		t.Fatalf("second Release should be a no-op: %v", err)
	}

	h2, err := Acquire(dir, OpStop)
	if err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
	h2.Release()
}

func TestAcquireBlocksWhileHeldByLiveProcess(t *testing.T) {
	dir := t.TempDir()

	h, err := Acquire(dir, OpStop)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer h.Release()

	_, err = Acquire(dir, OpStart)
	var held *HeldError
	if !errors.As(err, &held) {
		t.Fatalf("second Acquire error = %v, want *HeldError", err)
	}
	if held.Holder.PID != os.Getpid() || held.Holder.Op != "stop" {
		t.Fatalf("HeldError holder = %+v, want this pid doing stop", held.Holder)
	}
}

func TestAcquireReclaimsStaleLockFromDeadProcess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, lockName)

	body, err := toml.Marshal(Holder{PID: deadPID(t), Op: "start", Since: time.Now().Add(-time.Hour)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("seed stale lock: %v", err)
	}

	h, err := Acquire(dir, OpForceKill)
	if err != nil {
		t.Fatalf("Acquire over stale lock: %v", err)
	}
	defer h.Release()

	holder, err := readHolder(path)
	if err != nil {
		t.Fatalf("readHolder: %v", err)
	}
	if holder.PID != os.Getpid() {
		t.Fatalf("stale lock not reclaimed; holder pid = %d", holder.PID)
	}
}

func TestAcquireReclaimsUnparseableLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, lockName)
	if err := os.WriteFile(path, []byte("not toml at all"), 0o600); err != nil {
		t.Fatalf("seed junk lock: %v", err)
	}

	h, err := Acquire(dir, OpImport)
	if err != nil {
		t.Fatalf("Acquire over junk lock: %v", err)
	}
	h.Release()
}

// deadPID returns a PID that is not running. It spawns /bin/true, waits for it
// to exit, and returns its PID, which the OS will not have recycled by the time
// the test checks.
func deadPID(t *testing.T) int {
	t.Helper()
	p, err := os.StartProcess("/usr/bin/true", []string{"true"}, &os.ProcAttr{})
	if err != nil {
		p, err = os.StartProcess("/bin/true", []string{"true"}, &os.ProcAttr{})
	}
	if err != nil {
		t.Fatalf("spawn throwaway process: %v", err)
	}
	state, err := p.Wait()
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if !state.Exited() {
		t.Fatalf("throwaway process did not exit")
	}
	return p.Pid
}
