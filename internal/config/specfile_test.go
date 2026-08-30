package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sunyupei/beacon-tui/internal/server"
)

func TestLoadSpecsGoodFixture(t *testing.T) {
	specs, err := LoadSpecs(fixture("good"))
	if err != nil {
		t.Fatalf("LoadSpecs: %v", err)
	}
	want := []server.Spec{
		{
			ID:      "creative",
			Dir:     "/srv/minecraft/creative",
			Start:   "java -Xmx2G -jar paper.jar nogui",
			Port:    25566,
			Session: "beacon-creative",
			LogFile: "/srv/minecraft/creative/logs/beacon.log",
			Exec:    server.ExecMissing,
			State: server.State{
				LastKnown: server.StatusUnknown,
				PID:       4711,
				UpdatedAt: time.Date(2026, 8, 29, 23, 15, 30, 0, time.UTC),
			},
		},
		{
			ID:      "survival",
			Dir:     "/srv/minecraft/survival",
			Start:   "./run.sh",
			Script:  "run.sh",
			Port:    25565,
			Session: "beacon-survival",
			LogFile: "/srv/minecraft/survival/logs/beacon.log",
			Exec:    server.ExecOK,
			RCON:    server.RCON{Enabled: true, Port: 25575, Password: "hunter2"},
			State: server.State{
				LastKnown: server.StatusStopped,
				UpdatedAt: time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC),
			},
		},
	}
	if len(specs) != len(want) {
		t.Fatalf("LoadSpecs returned %d specs, want %d", len(specs), len(want))
	}
	for i := range want {
		if !sameSpec(specs[i], want[i]) {
			t.Errorf("spec %d = %+v, want %+v", i, specs[i], want[i])
		}
	}
}

// sameSpec compares UpdatedAt by instant, since the TOML decoder is free to pick
// any equivalent time.Location for the same offset.
func sameSpec(a, b server.Spec) bool {
	if !a.State.UpdatedAt.Equal(b.State.UpdatedAt) {
		return false
	}
	a.State.UpdatedAt = time.Time{}
	b.State.UpdatedAt = time.Time{}
	return reflect.DeepEqual(a, b)
}

func TestLoadSpecsMissingDir(t *testing.T) {
	specs, err := LoadSpecs(Dirs{Config: t.TempDir()})
	if err != nil {
		t.Fatalf("LoadSpecs: %v", err)
	}
	if specs != nil {
		t.Fatalf("LoadSpecs = %v, want nil", specs)
	}
}

func TestLoadSpecsRejectsFixtures(t *testing.T) {
	for _, name := range []string{"bad-status", "bad-exec-state"} {
		t.Run(name, func(t *testing.T) {
			dirs := fixture(name)
			_, err := LoadSpecs(dirs)
			if err == nil {
				t.Fatal("LoadSpecs accepted the fixture")
			}
			path := filepath.Join(dirs.ServersDir(), "broken.toml")
			if !strings.Contains(err.Error(), path) {
				t.Fatalf("error %q does not name %q", err, path)
			}
		})
	}
}

func TestLoadSpecsRejectsIDFilenameMismatch(t *testing.T) {
	dirs := Dirs{Config: t.TempDir()}
	if err := os.MkdirAll(dirs.ServersDir(), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(fixture("good").ServersDir(), "survival.toml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	path := filepath.Join(dirs.ServersDir(), "creative.toml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := LoadSpecs(dirs); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("LoadSpecs = %v, want an error naming %q", err, path)
	}
}

func TestLoadSpecRejectsUnknownKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "survival.toml")
	body := "id = \"survival\"\ndir = \"/srv/minecraft/survival\"\nstart = \"./run.sh\"\nport = 25565\n" +
		"session = \"beacon-survival\"\nlog_file = \"/srv/minecraft/survival/logs/beacon.log\"\nexec_state = \"ok\"\nprot = 25565\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := LoadSpec(path); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("LoadSpec = %v, want an error naming %q", err, path)
	}
}

func TestSpecRoundTrip(t *testing.T) {
	for _, name := range []string{"survival", "creative"} {
		t.Run(name, func(t *testing.T) {
			src := filepath.Join(fixture("good").ServersDir(), name+".toml")
			original, err := LoadSpec(src)
			if err != nil {
				t.Fatalf("LoadSpec: %v", err)
			}

			dirs := Dirs{Config: t.TempDir()}
			if err := SaveSpec(dirs, original); err != nil {
				t.Fatalf("SaveSpec: %v", err)
			}

			saved := dirs.ServerFile(original.ID)
			info, err := os.Stat(saved)
			if err != nil {
				t.Fatalf("Stat: %v", err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
			}

			reloaded, err := LoadSpec(saved)
			if err != nil {
				t.Fatalf("LoadSpec after SaveSpec: %v", err)
			}
			if !reflect.DeepEqual(original, reloaded) {
				t.Fatalf("round trip changed the spec:\n before %+v\n after  %+v", original, reloaded)
			}
		})
	}
}
