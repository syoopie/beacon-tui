package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultDirsState(t *testing.T) {
	home := t.TempDir()

	tests := []struct {
		name string
		xdg  string
		want string
	}{
		{"xdg set", filepath.Join(home, "xdgstate"), filepath.Join(home, "xdgstate", "beacon")},
		{"xdg unset", "", filepath.Join(home, ".local", "state", "beacon")},
		{"xdg relative", "relative/state", filepath.Join(home, ".local", "state", "beacon")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv("XDG_STATE_HOME", tt.xdg)

			dirs, err := DefaultDirs()
			if err != nil {
				t.Fatalf("DefaultDirs: %v", err)
			}
			if dirs.State != tt.want {
				t.Fatalf("State = %q, want %q", dirs.State, tt.want)
			}
			if !strings.HasPrefix(dirs.Config, home) || filepath.Base(dirs.Config) != "beacon" {
				t.Fatalf("Config = %q, want a beacon directory under %q", dirs.Config, home)
			}
		})
	}
}

func TestDirsPaths(t *testing.T) {
	d := Dirs{Config: "/cfg/beacon", State: "/state/beacon"}
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"ConfigFile", d.ConfigFile(), "/cfg/beacon/config.toml"},
		{"ServersDir", d.ServersDir(), "/cfg/beacon/servers"},
		{"LogsDir", d.LogsDir(), "/state/beacon/logs"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}
