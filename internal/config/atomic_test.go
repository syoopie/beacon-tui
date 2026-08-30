package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	path := filepath.Join(dir, "secret.toml")

	if err := WriteFileAtomic(path, []byte("first"), 0o600); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	assertFile(t, path, "first", 0o600)

	if err := WriteFileAtomic(path, []byte("second"), 0o600); err != nil {
		t.Fatalf("WriteFileAtomic overwrite: %v", err)
	}
	assertFile(t, path, "second", 0o600)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "secret.toml" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Fatalf("directory holds %v, want only secret.toml", names)
	}
}

func TestWriteFileAtomicMode(t *testing.T) {
	dir := t.TempDir()
	for _, perm := range []os.FileMode{0o600, 0o644} {
		path := filepath.Join(dir, perm.String()+".toml")
		if err := WriteFileAtomic(path, []byte("x"), perm); err != nil {
			t.Fatalf("WriteFileAtomic(%v): %v", perm, err)
		}
		assertFile(t, path, "x", perm)
	}
}

func assertFile(t *testing.T, path, want string, perm os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != want {
		t.Fatalf("content = %q, want %q", data, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != perm {
		t.Fatalf("mode = %v, want %v", info.Mode().Perm(), perm)
	}
}
