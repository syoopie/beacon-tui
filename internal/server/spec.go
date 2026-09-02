package server

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"time"
)

// Spec is one server as recorded in servers/<id>.toml. Parsed once at the
// boundary; the rest of the program trusts these types.
type Spec struct {
	ID      ID        `toml:"id"`
	Dir     string    `toml:"dir"`    // absolute path to the server directory
	Start   string    `toml:"start"`  // shell command run inside Dir
	Script  string    `toml:"script"` // start script relative to Dir, empty when launching a jar directly
	Java    string    `toml:"java"`   // absolute path to a java executable; empty means the java on PATH
	Port    int       `toml:"port"`
	Session Session   `toml:"session"`
	LogFile string    `toml:"log_file"` // absolute
	Exec    ExecState `toml:"exec_state"`
	RCON    RCON      `toml:"rcon"`
	State   State     `toml:"state"`

	Commands Commands `toml:"commands"`
}

// Commands configures the console's command completion for this server. A spec
// written before this block existed decodes with the zero value, which means
// "auto": use the bundled command tree for MCVersion, or fall back to plain
// input when MCVersion is unknown. importdetect fills MCVersion and Loader at
// import; the operator can correct either by hand.
type Commands struct {
	MCVersion  string `toml:"mc_version"` // "1.20.1"; blank when detection could not tell
	Loader     string `toml:"loader"`     // vanilla|forge|neoforge|fabric|quilt|paper|purpur|folia|spigot|craftbukkit; informational until the RCON phase
	Completion string `toml:"completion"` // "" or "auto" (default), or "off"
}

var (
	mcVersionRe  = regexp.MustCompile(`^[0-9]+\.[0-9]+(\.[0-9]+)?$`)
	knownLoaders = []string{
		"vanilla", "forge", "neoforge", "fabric", "quilt",
		"paper", "purpur", "folia", "spigot", "craftbukkit",
	}
)

func (c Commands) validate() error {
	switch c.Completion {
	case "", "auto", "off":
	default:
		return fmt.Errorf("completion %q: want \"auto\" or \"off\"", c.Completion)
	}
	if c.MCVersion != "" && !mcVersionRe.MatchString(c.MCVersion) {
		return fmt.Errorf("mc_version %q: want a Minecraft version like 1.20.1", c.MCVersion)
	}
	if c.Loader != "" && !slices.Contains(knownLoaders, c.Loader) {
		return fmt.Errorf("loader %q: not a known server type", c.Loader)
	}
	return nil
}

// CompletionEnabled reports whether the console should offer command completion
// for this server. It is off only when the operator set it so.
func (c Commands) CompletionEnabled() bool { return c.Completion != "off" }

// ValidMCVersion reports whether s is a Minecraft version string Beacon accepts,
// like "1.20" or "1.20.1". The empty string is not valid here; callers that
// allow "unset" check for that themselves.
func ValidMCVersion(s string) bool { return mcVersionRe.MatchString(s) }

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
	if s.Java != "" && !filepath.IsAbs(s.Java) {
		return fmt.Errorf("java %q: not an absolute path", s.Java)
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
	if err := s.Commands.validate(); err != nil {
		return fmt.Errorf("commands: %w", err)
	}
	return nil
}

func validPort(p int) bool { return p >= 1 && p <= 65535 }
