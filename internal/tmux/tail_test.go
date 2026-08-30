package tmux

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func newTestTailer(t *testing.T, path string) *Tailer {
	t.Helper()
	tl := NewTailer(path)
	t.Cleanup(func() { _ = tl.Close() })
	return tl
}

func appendTo(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	if _, err := f.WriteString(s); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
}

func read(t *testing.T, tl *Tailer) []string {
	t.Helper()
	lines, err := tl.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	return lines
}

func TestTailerReadsEachLineOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	tl := newTestTailer(t, path)

	appendTo(t, path, "one\ntwo\n")
	if got := read(t, tl); !slices.Equal(got, []string{"one", "two"}) {
		t.Fatalf("Read = %q, want [one two]", got)
	}
	if got := read(t, tl); len(got) != 0 {
		t.Fatalf("Read with no new bytes = %q, want nothing", got)
	}

	appendTo(t, path, "three\n")
	if got := read(t, tl); !slices.Equal(got, []string{"three"}) {
		t.Fatalf("Read = %q, want [three]", got)
	}
}

func TestTailerWithholdsPartialLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	tl := newTestTailer(t, path)

	appendTo(t, path, "half")
	if got := read(t, tl); len(got) != 0 {
		t.Fatalf("Read of an unterminated line = %q, want nothing", got)
	}

	appendTo(t, path, " and half\n")
	if got := read(t, tl); !slices.Equal(got, []string{"half and half"}) {
		t.Fatalf("Read = %q, want [half and half]", got)
	}
}

func TestTailerReopensAfterTruncate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	tl := newTestTailer(t, path)

	appendTo(t, path, "before one\nbefore two\n")
	if got := read(t, tl); !slices.Equal(got, []string{"before one", "before two"}) {
		t.Fatalf("Read = %q, want [before one, before two]", got)
	}

	if err := os.Truncate(path, 0); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	appendTo(t, path, "after\n")
	if got := read(t, tl); !slices.Equal(got, []string{"after"}) {
		t.Fatalf("Read after truncate = %q, want [after]", got)
	}
}

func TestTailerReopensAfterReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	tl := newTestTailer(t, path)

	appendTo(t, path, "original\n")
	if got := read(t, tl); !slices.Equal(got, []string{"original"}) {
		t.Fatalf("Read = %q, want [original]", got)
	}

	staged := filepath.Join(dir, "log.staged")
	if err := os.WriteFile(staged, []byte("replacement\n"), 0o600); err != nil {
		t.Fatalf("write staged file: %v", err)
	}
	if err := os.Rename(staged, path); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got := read(t, tl); !slices.Equal(got, []string{"replacement"}) {
		t.Fatalf("Read after replace = %q, want [replacement]", got)
	}
}

func TestTailerReopensAfterCloseOntoAShorterFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	tl := newTestTailer(t, path)

	appendTo(t, path, "a first line long enough to advance the offset\n")
	if got := read(t, tl); !slices.Equal(got, []string{"a first line long enough to advance the offset"}) {
		t.Fatalf("Read = %q", got)
	}
	if err := tl.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := os.WriteFile(path, []byte("short\n"), 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if got := read(t, tl); !slices.Equal(got, []string{"short"}) {
		t.Fatalf("Read after reopening onto a shorter file = %q, want [short]", got)
	}
}

func TestTailerMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-yet")
	tl := newTestTailer(t, path)

	lines, err := tl.Read()
	if err != nil {
		t.Fatalf("Read of a missing path: %v", err)
	}
	if lines != nil {
		t.Fatalf("Read of a missing path = %q, want nil", lines)
	}

	appendTo(t, path, "appeared\n")
	if got := read(t, tl); !slices.Equal(got, []string{"appeared"}) {
		t.Fatalf("Read after the file appeared = %q, want [appeared]", got)
	}
}

func TestTailerCarriageReturnAndFinalPartialLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	tl := newTestTailer(t, path)

	appendTo(t, path, "progress\rdone\nno newline yet")
	if got := read(t, tl); !slices.Equal(got, []string{"progress\rdone"}) {
		t.Fatalf("Read = %q, want [progress\\rdone]", got)
	}
	if got := read(t, tl); len(got) != 0 {
		t.Fatalf("Read at EOF on an unterminated line = %q, want nothing", got)
	}

	appendTo(t, path, "\n")
	if got := read(t, tl); !slices.Equal(got, []string{"no newline yet"}) {
		t.Fatalf("Read = %q, want [no newline yet]", got)
	}
}
