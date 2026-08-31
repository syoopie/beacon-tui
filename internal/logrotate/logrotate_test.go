package logrotate

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	body := strings.Repeat("a line of server log output\n", size/28+1)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRotateLeavesASmallFileAlone(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "survival.log")
	writeFile(t, log, 100)

	rotated, err := Rotate(log, Policy{MaxLiveBytes: 1 << 20, BudgetBytes: 1 << 30})
	if err != nil {
		t.Fatal(err)
	}
	if rotated {
		t.Fatal("a small file should not rotate")
	}
	if _, err := os.Stat(log); err != nil {
		t.Fatalf("the live file should be untouched: %v", err)
	}
}

func TestRotateArchivesAndTruncates(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "survival.log")
	writeFile(t, log, 4000)
	original, _ := os.ReadFile(log)

	rotated, err := Rotate(log, Policy{MaxLiveBytes: 1000, BudgetBytes: 1 << 30})
	if err != nil {
		t.Fatal(err)
	}
	if !rotated {
		t.Fatal("a file over the limit should rotate")
	}

	info, err := os.Stat(log)
	if err != nil || info.Size() != 0 {
		t.Fatalf("the live file should be truncated to zero, got size %d err %v", info.Size(), err)
	}

	archives, _ := filepath.Glob(filepath.Join(dir, "survival.*.log.gz"))
	if len(archives) != 1 {
		t.Fatalf("want one archive, got %v", archives)
	}
	if got := gunzip(t, archives[0]); got != string(original) {
		t.Fatalf("archive content does not match the original log")
	}
}

func TestRotateMissingFileIsNotAnError(t *testing.T) {
	rotated, err := Rotate(filepath.Join(t.TempDir(), "nope.log"), DefaultPolicy)
	if err != nil || rotated {
		t.Fatalf("want (false, nil), got (%v, %v)", rotated, err)
	}
}

func TestRotatePrunesOldestPastBudget(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "survival.log")

	// Three archives, ~1 KB each, oldest to newest by timestamp in the name.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var names []string
	for i := 0; i < 3; i++ {
		name := archiveName(log, base.Add(time.Duration(i)*time.Hour))
		if err := os.WriteFile(name, make([]byte, 1024), 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	writeFile(t, log, 4000)

	// Budget fits two archives, so the oldest is pruned on rotation. The new
	// archive plus the two newest kept ones is still three files.
	if _, err := Rotate(log, Policy{MaxLiveBytes: 1000, BudgetBytes: 3 * 1024}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(names[0]); !os.IsNotExist(err) {
		t.Fatalf("the oldest archive should have been pruned, stat err = %v", err)
	}
	for _, keep := range names[1:] {
		if _, err := os.Stat(keep); err != nil {
			t.Fatalf("a newer archive was pruned: %v", err)
		}
	}
	left, _ := filepath.Glob(filepath.Join(dir, "survival.*.log.gz"))
	if len(left) != 3 {
		t.Fatalf("want 3 archives after rotate, got %d", len(left))
	}
}

func TestDue(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "survival.log")
	writeFile(t, log, 4000)

	if Due(log, Policy{MaxLiveBytes: 1 << 20}) {
		t.Fatal("a 4 KB file is not due against a 1 MiB limit")
	}
	if !Due(log, Policy{MaxLiveBytes: 1000}) {
		t.Fatal("a 4 KB file is due against a 1 KB limit")
	}
	if Due(filepath.Join(dir, "missing.log"), DefaultPolicy) {
		t.Fatal("a missing file is never due")
	}
}

func gunzip(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
