// Package supervisor is the port beacon uses to own server process lifetime.
// It names no supervisor technology, so a future Windows adapter can satisfy it
// without importing internal/tmux.
package supervisor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sunyupei/beacon-tui/internal/server"
)

// Launch is everything needed to bring one server up under a supervisor.
type Launch struct {
	Session server.Session
	Dir     string // absolute working directory
	Command string // shell command; the supervisor must exec it so it owns the PID
	LogFile string // absolute; stdout and stderr are appended here
}

// Supervisor owns process lifetime and stdin. It does not own logs, config, or
// status: logs are files, and the session's existence is the only truth it reports.
type Supervisor interface {
	// Start is idempotent. An existing session is left alone rather than joined
	// by a second process.
	Start(ctx context.Context, l Launch) error
	Exists(ctx context.Context, s server.Session) (bool, error)
	SendKeys(ctx context.Context, s server.Session, line string) error
	// Kill returns nil when the session is already gone.
	Kill(ctx context.Context, s server.Session) error
}

// Validate is the boundary check on a Launch built from a Spec.
func (l Launch) Validate() error {
	if l.Session == "" {
		return fmt.Errorf("session: empty")
	}
	if !strings.HasPrefix(string(l.Session), server.SessionPrefix) {
		return fmt.Errorf("session %q: must start with %q", l.Session, server.SessionPrefix)
	}
	if l.Dir == "" {
		return fmt.Errorf("dir: empty")
	}
	if !filepath.IsAbs(l.Dir) {
		return fmt.Errorf("dir %q: not an absolute path", l.Dir)
	}
	if l.LogFile == "" {
		return fmt.Errorf("log_file: empty")
	}
	if !filepath.IsAbs(l.LogFile) {
		return fmt.Errorf("log_file %q: not an absolute path", l.LogFile)
	}
	if l.Command == "" {
		return fmt.Errorf("command: empty")
	}
	if strings.ContainsAny(l.Command, "\n\r") {
		return fmt.Errorf("command %q: contains a newline", l.Command)
	}
	return nil
}
