package server

import (
	"fmt"
	"path/filepath"
	"time"
)

// Spec is one server as recorded in servers/<id>.toml. Parsed once at the
// boundary; the rest of the program trusts these types.
type Spec struct {
	ID      ID        `toml:"id"`
	Dir     string    `toml:"dir"`    // absolute path to the server directory
	Start   string    `toml:"start"`  // shell command run inside Dir
	Script  string    `toml:"script"` // start script relative to Dir, empty when launching a jar directly
	Port    int       `toml:"port"`
	Session Session   `toml:"session"`
	LogFile string    `toml:"log_file"` // absolute
	Exec    ExecState `toml:"exec_state"`
	RCON    RCON      `toml:"rcon"`
	State   State     `toml:"state"`
}

// RCON mirrors server.properties. The password is plaintext on disk, which is
// why spec files are 0600.
type RCON struct {
	Enabled  bool   `toml:"enabled"`
	Port     int    `toml:"port"`
	Password string `toml:"password"`
}

// State is a cache for the next boot, not truth. tmux is truth. Written only
// under the host op lock (phase 6).
type State struct {
	LastKnown Status    `toml:"last_known"`
	PID       int       `toml:"pid"`
	UpdatedAt time.Time `toml:"updated_at"`
}

// Validate is the boundary check for a spec that came off disk.
func (s Spec) Validate() error {
	if _, err := ParseID(string(s.ID)); err != nil {
		return fmt.Errorf("id: %w", err)
	}
	if s.Dir == "" {
		return fmt.Errorf("dir: empty")
	}
	if !filepath.IsAbs(s.Dir) {
		return fmt.Errorf("dir %q: not an absolute path", s.Dir)
	}
	if s.Start == "" {
		return fmt.Errorf("start: empty")
	}
	if !validPort(s.Port) {
		return fmt.Errorf("port %d: outside 1..65535", s.Port)
	}
	if err := s.Session.validate(); err != nil {
		return fmt.Errorf("session: %w", err)
	}
	if s.LogFile == "" {
		return fmt.Errorf("log_file: empty")
	}
	if !filepath.IsAbs(s.LogFile) {
		return fmt.Errorf("log_file %q: not an absolute path", s.LogFile)
	}
	if s.RCON.Port != 0 && !validPort(s.RCON.Port) {
		return fmt.Errorf("rcon.port %d: outside 1..65535", s.RCON.Port)
	}
	if s.State.PID < 0 {
		return fmt.Errorf("state.pid %d: negative", s.State.PID)
	}
	return nil
}

func validPort(p int) bool { return p >= 1 && p <= 65535 }
