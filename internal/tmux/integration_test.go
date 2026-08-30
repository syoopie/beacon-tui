//go:build unix

package tmux

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/syoopie/beacon-tui/internal/server"
	"github.com/syoopie/beacon-tui/internal/supervisor"
)

const bootLine = "boot"

// missingBin is a tmux path that cannot be executed, so a test asserting tmux is
// never invoked fails loudly if it is.
func missingBin(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "tmux-must-not-run")
}

func testSession(t *testing.T, name string) server.Session {
	t.Helper()
	id, err := server.ParseID("test-" + strconv.Itoa(os.Getpid()) + "-" + name)
	if err != nil {
		t.Fatalf("ParseID: %v", err)
	}
	return server.SessionFor(id)
}

func liveClient(t *testing.T) *Client {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not on PATH")
	}
	return &Client{}
}

// liveSession names a session and guarantees it is gone when the test ends, so a
// failure never strands a session on the operator's tmux server.
func liveSession(t *testing.T, c *Client, name string) server.Session {
	t.Helper()
	s := testSession(t, name)
	t.Cleanup(func() {
		if err := c.Kill(context.Background(), s); err != nil {
			t.Errorf("cleanup kill %s: %v", s, err)
		}
	})
	return s
}

func panePID(t *testing.T, c *Client, s server.Session) int {
	t.Helper()
	out, err := c.run(context.Background(), "list-panes", "-t", target(s), "-F", "#{pane_pid}")
	if err != nil {
		t.Fatalf("list-panes: %v", err)
	}
	fields := strings.Fields(out)
	if len(fields) != 1 {
		t.Fatalf("list-panes printed %q, want exactly one pane pid", out)
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatalf("pane pid %q: %v", fields[0], err)
	}
	return pid
}

func processName(t *testing.T, pid int) string {
	t.Helper()
	out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		t.Fatalf("ps for pid %d: %v", pid, err)
	}
	// macOS prints the full executable path under comm.
	return filepath.Base(strings.TrimSpace(string(out)))
}

func waitForLine(t *testing.T, tl *Tailer, want string) {
	t.Helper()
	var seen []string
	deadline := time.Now().Add(5 * time.Second)
	for {
		lines, err := tl.Read()
		if err != nil {
			t.Fatalf("tail: %v", err)
		}
		seen = append(seen, lines...)
		if slices.Contains(seen, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("log never yielded %q; saw %q", want, seen)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestClientEndToEnd(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()
	s := liveSession(t, c, "e2e")

	dir := t.TempDir()
	l := supervisor.Launch{
		Session: s,
		Dir:     dir,
		Command: "sh -c 'echo " + bootLine + "; exec sleep 30'",
		LogFile: filepath.Join(dir, "logs", "server.log"),
	}
	if err := c.Start(ctx, l); err != nil {
		t.Fatalf("Start: %v", err)
	}

	exists, err := c.Exists(ctx, s)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Fatal("Exists = false immediately after Start returned")
	}

	tl := newTestTailer(t, l.LogFile)
	waitForLine(t, tl, bootLine)

	pid := panePID(t, c, s)
	if name := processName(t, pid); name != "sleep" {
		t.Fatalf("pane process = %q, want %q: the launch script did not exec the command", name, "sleep")
	}

	if err := c.Start(ctx, l); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if again := panePID(t, c, s); again != pid {
		t.Fatalf("pane pid moved from %d to %d: the second Start added a process", pid, again)
	}

	if err := c.SendKeys(ctx, s, "status"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}

	if err := c.Kill(ctx, s); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	exists, err = c.Exists(ctx, s)
	if err != nil {
		t.Fatalf("Exists after Kill: %v", err)
	}
	if exists {
		t.Fatal("Exists = true after Kill")
	}
	if err := c.Kill(ctx, s); err != nil {
		t.Fatalf("Kill of an already dead session: %v", err)
	}
}

func TestSendKeysRejectsEmbeddedNewline(t *testing.T) {
	c := &Client{Bin: missingBin(t)}
	err := c.SendKeys(context.Background(), testSession(t, "newline"), "first\nsecond")
	if err == nil {
		t.Fatal("SendKeys accepted a line containing a newline")
	}
	if !strings.Contains(err.Error(), "newline") {
		t.Fatalf("SendKeys error = %v, want the rejection rather than a failed exec", err)
	}
}

func TestExistsUnknownSession(t *testing.T) {
	c := liveClient(t)
	s := testSession(t, "absent")

	exists, err := c.Exists(context.Background(), s)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Fatalf("Exists(%s) = true for a session that was never created", s)
	}
}

func TestStartRejectsInvalidLaunch(t *testing.T) {
	c := &Client{Bin: missingBin(t)}
	dir := t.TempDir()
	err := c.Start(context.Background(), supervisor.Launch{
		Session: testSession(t, "invalid"),
		Dir:     "relative/dir",
		Command: "true",
		LogFile: filepath.Join(dir, "server.log"),
	})
	if err == nil {
		t.Fatal("Start accepted a Launch with a relative Dir")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("Start error = %v, want Validate's absolute-path failure", err)
	}
}
