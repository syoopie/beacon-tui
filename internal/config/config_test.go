package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fixture(name string) Dirs {
	return Dirs{Config: filepath.Join("..", "..", "testdata", "config", name)}
}

func TestLoadGoodFixture(t *testing.T) {
	c, err := Load(fixture("good"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		ScanRoots:   []string{"/srv/minecraft", "/opt/minecraft"},
		StopTimeout: Duration(90 * time.Second),
	}
	if !reflect.DeepEqual(c, want) {
		t.Fatalf("Load = %+v, want %+v", c, want)
	}
	if c.StopTimeout.Std() != 90*time.Second {
		t.Fatalf("Std = %v", c.StopTimeout.Std())
	}
}

func TestLoadMissingFile(t *testing.T) {
	dirs := Dirs{Config: t.TempDir()}
	_, err := Load(dirs)
	if !errors.Is(err, ErrNoConfig) {
		t.Fatalf("Load = %v, want ErrNoConfig", err)
	}
	if !strings.Contains(err.Error(), dirs.ConfigFile()) {
		t.Fatalf("error %q does not name %q", err, dirs.ConfigFile())
	}
}

func TestLoadAllowsEmptyScanRoots(t *testing.T) {
	c, err := Load(fixture("no-scan-roots"))
	if err != nil {
		t.Fatalf("Load rejected a config with no scan roots: %v", err)
	}
	if len(c.ScanRoots) != 0 {
		t.Fatalf("ScanRoots = %v, want empty", c.ScanRoots)
	}
}

func TestAddScanRoot(t *testing.T) {
	dirs := Dirs{Config: t.TempDir()}
	target := t.TempDir()

	c, err := AddScanRoot(dirs, target)
	if err != nil {
		t.Fatalf("AddScanRoot: %v", err)
	}
	if len(c.ScanRoots) != 1 || c.ScanRoots[0] != target {
		t.Fatalf("ScanRoots = %v, want [%s]", c.ScanRoots, target)
	}

	again, err := AddScanRoot(dirs, target)
	if err != nil {
		t.Fatalf("AddScanRoot (repeat): %v", err)
	}
	if len(again.ScanRoots) != 1 {
		t.Fatalf("adding the same root twice produced %v", again.ScanRoots)
	}

	reloaded, err := Load(dirs)
	if err != nil {
		t.Fatalf("Load after AddScanRoot: %v", err)
	}
	if len(reloaded.ScanRoots) != 1 || reloaded.ScanRoots[0] != target {
		t.Fatalf("persisted ScanRoots = %v", reloaded.ScanRoots)
	}
}

func TestAddScanRootRejectsNonDirectory(t *testing.T) {
	dirs := Dirs{Config: t.TempDir()}
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := AddScanRoot(dirs, file); err == nil {
		t.Fatal("AddScanRoot accepted a path that is not a directory")
	}
}

func TestLoadRejectsFixtures(t *testing.T) {
	for _, name := range []string{"unknown-key"} {
		t.Run(name, func(t *testing.T) {
			dirs := fixture(name)
			_, err := Load(dirs)
			if err == nil {
				t.Fatal("Load accepted the fixture")
			}
			if !strings.Contains(err.Error(), dirs.ConfigFile()) {
				t.Fatalf("error %q does not name %q", err, dirs.ConfigFile())
			}
		})
	}
}

func TestLoadInlineRejects(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"relative scan root", "scan_roots = [\"minecraft\"]\n"},
		{"zero stop timeout", "scan_roots = [\"/srv\"]\nstop_timeout = \"0s\"\n"},
		{"negative stop timeout", "scan_roots = [\"/srv\"]\nstop_timeout = \"-5s\"\n"},
		{"unparseable stop timeout", "scan_roots = [\"/srv\"]\nstop_timeout = \"soon\"\n"},
		{"malformed toml", "scan_roots = [\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dirs := writeConfig(t, tt.body)
			_, err := Load(dirs)
			if err == nil {
				t.Fatal("Load accepted the config")
			}
			if !strings.Contains(err.Error(), dirs.ConfigFile()) {
				t.Fatalf("error %q does not name %q", err, dirs.ConfigFile())
			}
		})
	}
}

func TestLoadDefaultsStopTimeout(t *testing.T) {
	dirs := writeConfig(t, "scan_roots = [\"/srv/minecraft\"]\n")
	c, err := Load(dirs)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.StopTimeout.Std() != 60*time.Second {
		t.Fatalf("StopTimeout = %v, want 60s", c.StopTimeout.Std())
	}
}

func TestSaveRoundTrip(t *testing.T) {
	dirs := Dirs{Config: t.TempDir()}
	want := Config{
		ScanRoots:   []string{"/srv/minecraft"},
		StopTimeout: Duration(90 * time.Second),
	}
	if err := Save(dirs, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(dirs.ConfigFile())
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %v, want 0644", info.Mode().Perm())
	}

	got, err := Load(dirs)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func TestDurationText(t *testing.T) {
	for _, d := range []Duration{Duration(60 * time.Second), Duration(90 * time.Second), Duration(2 * time.Hour)} {
		text, err := d.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText: %v", err)
		}
		var back Duration
		if err := back.UnmarshalText(text); err != nil {
			t.Fatalf("UnmarshalText(%q): %v", text, err)
		}
		if back != d {
			t.Fatalf("round trip of %v yielded %v", d.Std(), back.Std())
		}
	}
	var d Duration
	if err := d.UnmarshalText([]byte("soon")); err == nil {
		t.Fatal("UnmarshalText accepted \"soon\"")
	}
}

func writeConfig(t *testing.T, body string) Dirs {
	t.Helper()
	dirs := Dirs{Config: t.TempDir()}
	if err := os.WriteFile(dirs.ConfigFile(), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return dirs
}
