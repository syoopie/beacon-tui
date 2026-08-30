package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/syoopie/beacon-tui/internal/server"
)

// Dirs are the resolved on-disk locations beacon reads and writes.
type Dirs struct {
	Config string // e.g. ~/Library/Application Support/beacon
	State  string // e.g. ~/.local/state/beacon
}

// DefaultDirs resolves Dirs from the OS. Go has no os.UserStateDir, so state
// follows XDG on every platform, including Darwin.
func DefaultDirs() (Dirs, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return Dirs{}, fmt.Errorf("resolving the config directory: %w", err)
	}

	state := os.Getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(state) {
		home, err := os.UserHomeDir()
		if err != nil {
			return Dirs{}, fmt.Errorf("resolving the state directory: %w", err)
		}
		state = filepath.Join(home, ".local", "state")
	}

	return Dirs{
		Config: filepath.Join(cfg, "beacon"),
		State:  filepath.Join(state, "beacon"),
	}, nil
}

func (d Dirs) ConfigFile() string { return filepath.Join(d.Config, "config.toml") }

func (d Dirs) ServersDir() string { return filepath.Join(d.Config, "servers") }

func (d Dirs) ServerFile(id server.ID) string {
	return filepath.Join(d.ServersDir(), id.String()+".toml")
}

func (d Dirs) LogsDir() string { return filepath.Join(d.State, "logs") }

func (d Dirs) LogFile(id server.ID) string {
	return filepath.Join(d.LogsDir(), id.String()+".log")
}
