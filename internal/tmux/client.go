// Package tmux is beacon's first supervisor adapter. It drives the tmux CLI and
// keeps every tmux-specific spelling out of the rest of the program.
package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sunyupei/beacon-tui/internal/server"
	"github.com/sunyupei/beacon-tui/internal/supervisor"
)

// Client drives the tmux CLI.
type Client struct {
	Bin string // tmux binary; "tmux" when empty
}

var _ supervisor.Supervisor = (*Client)(nil)

// exitError carries tmux's exit status, because tmux answers "no such session"
// with status 1 on every command and beacon must not read that as a failure.
type exitError struct {
	code   int
	stderr string
}

func (e *exitError) Error() string {
	if e.stderr == "" {
		return fmt.Sprintf("exit status %d", e.code)
	}
	return fmt.Sprintf("exit status %d: %s", e.code, e.stderr)
}

func asExitError(err error) (*exitError, bool) {
	var e *exitError
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

func (c *Client) bin() string {
	if c.Bin == "" {
		return "tmux"
	}
	return c.Bin
}

func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	var stdout, stderr strings.Builder
	cmd := exec.CommandContext(ctx, c.bin(), args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return stdout.String(), nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		status := &exitError{code: ee.ExitCode(), stderr: strings.TrimSpace(stderr.String())}
		return stdout.String(), fmt.Errorf("tmux %s: %w", args[0], status)
	}
	return stdout.String(), fmt.Errorf("tmux %s: %w", args[0], err)
}

// target names a session for a command that takes a target-session. The "="
// forces an exact match; without it tmux prefix-matches, so a stray
// "beacon-web-2" would answer for "beacon-web".
func target(s server.Session) string { return "=" + string(s) }

// paneTarget names a session for a command that takes a target-pane. send-keys
// is one of those, and it rejects a bare "=<session>": the trailing colon is
// what makes the session's current pane a valid pane target.
func paneTarget(s server.Session) string { return "=" + string(s) + ":" }

// shellQuote wraps s for /bin/sh so a log path containing spaces or apostrophes
// survives the redirect.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (c *Client) Start(ctx context.Context, l supervisor.Launch) error {
	if err := l.Validate(); err != nil {
		return fmt.Errorf("launch: %w", err)
	}
	exists, err := c.Exists(ctx, l.Session)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(l.LogFile), 0o700); err != nil {
		return fmt.Errorf("log directory for %s: %w", l.Session, err)
	}

	// The first exec redirects the shell's own descriptors; the second replaces
	// the shell, so the pane PID is the server process itself. Command is left
	// unquoted on purpose: it is a shell command line, not a path.
	script := "exec >>" + shellQuote(l.LogFile) + " 2>&1\nexec " + l.Command

	_, err = c.run(ctx, "new-session", "-d", "-s", string(l.Session), "-c", l.Dir, "/bin/sh", "-c", script)
	if e, ok := asExitError(err); ok && e.code == 1 && strings.Contains(e.stderr, "duplicate session") {
		// Another beacon created the session between our Exists check and this
		// call. Its session is the one the caller asked for.
		return nil
	}
	return err
}

func (c *Client) Exists(ctx context.Context, s server.Session) (bool, error) {
	_, err := c.run(ctx, "has-session", "-t", target(s))
	if e, ok := asExitError(err); ok && e.code == 1 {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (c *Client) SendKeys(ctx context.Context, s server.Session, line string) error {
	if strings.ContainsAny(line, "\n\r") {
		return fmt.Errorf("send-keys to %s: line contains a newline", s)
	}
	t := paneTarget(s)
	// -l sends the text literally and -- keeps a leading '-' from being read as a
	// flag. Enter is a separate call so no character of line can be taken for a
	// key name.
	if _, err := c.run(ctx, "send-keys", "-t", t, "-l", "--", line); err != nil {
		return err
	}
	_, err := c.run(ctx, "send-keys", "-t", t, "Enter")
	return err
}

func (c *Client) Kill(ctx context.Context, s server.Session) error {
	_, err := c.run(ctx, "kill-session", "-t", target(s))
	if e, ok := asExitError(err); ok && e.code == 1 {
		return nil
	}
	return err
}
