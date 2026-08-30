package importdetect

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/sunyupei/beacon-tui/internal/config"
	"github.com/sunyupei/beacon-tui/internal/server"
)

// ExecCheck is the result of inspecting a start script's last effective command.
type ExecCheck struct {
	State server.ExecState
	Line  int    // 1-based; 0 when the script has no effective command
	Text  string // that line, verbatim
}

func InspectScript(path string) (ExecCheck, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ExecCheck{}, fmt.Errorf("reading %s: %w", path, err)
	}

	n, text := lastEffective(strings.Split(string(data), "\n"))
	if n == 0 {
		return ExecCheck{State: server.ExecMissing}, nil
	}
	return ExecCheck{State: execState(text), Line: n, Text: text}, nil
}

func lastEffective(lines []string) (int, string) {
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return i + 1, lines[i]
	}
	return 0, ""
}

// execState refuses everything but a literal exec of java. A bare `java -jar`,
// a wrapper script or a screen invocation may still hand the JVM the pane's PID,
// but beacon cannot prove it, so it refuses rather than guesses.
func execState(line string) server.ExecState {
	trimmed := strings.TrimSpace(line)
	fields := strings.Fields(trimmed)
	if len(fields) < 2 || fields[0] != "exec" {
		return server.ExecMissing
	}
	if !strings.Contains(strings.ToLower(trimmed), "java") {
		return server.ExecMissing
	}
	return server.ExecOK
}

// Patch is the one-line edit that hands the script's PID to Java.
type Patch struct {
	Path string
	Line int
	Old  string
	New  string
}

// Diff renders the confirmation prompt's one-line diff.
func (p Patch) Diff() string {
	return fmt.Sprintf("%s:%d\n-%s\n+%s", p.Path, p.Line, p.Old, p.New)
}

// PlanPatch returns the patch a script needs. ok is false when the script
// already execs, in which case Patch is zero.
func PlanPatch(path string) (p Patch, ok bool, err error) {
	check, err := InspectScript(path)
	if err != nil {
		return Patch{}, false, err
	}
	if check.State == server.ExecOK {
		return Patch{}, false, nil
	}
	if check.Line == 0 {
		return Patch{}, false, fmt.Errorf("patching %s: no command to patch", path)
	}

	indent := check.Text[:len(check.Text)-len(strings.TrimLeft(check.Text, " \t"))]
	return Patch{
		Path: path,
		Line: check.Line,
		Old:  check.Text,
		New:  indent + "exec " + strings.TrimSpace(check.Text),
	}, true, nil
}

// Apply rewrites the script after backing it up. The caller is responsible for
// getting the operator's explicit confirmation first.
func Apply(p Patch) error {
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", p.Path, err)
	}

	// Checked before anything is written, so a confirmation the operator has
	// since invalidated by editing the script cannot corrupt it.
	lines := strings.Split(string(data), "\n")
	if p.Line < 1 || p.Line > len(lines) || lines[p.Line-1] != p.Old {
		return fmt.Errorf("patching %s: line %d has changed since it was inspected", p.Path, p.Line)
	}

	info, err := os.Stat(p.Path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", p.Path, err)
	}
	mode := info.Mode().Perm()

	if err := writeBackup(p.Path+".bak", data, mode); err != nil {
		return err
	}

	lines[p.Line-1] = p.New
	return config.WriteFileAtomic(p.Path, []byte(strings.Join(lines, "\n")), mode)
}

// writeBackup leaves an existing .bak alone: the operator's pristine script is
// worth more than the copy from the most recent patch.
func writeBackup(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}

	// OpenFile's mode is masked by the umask, so a group-writable script would
	// come back narrower than the original. Chmod restores it exactly.
	// OpenFile's mode is masked by the umask, so a group-writable script would
	// come back narrower than the original. Chmod restores it exactly.
	err = f.Chmod(mode)
	if err == nil {
		_, err = f.Write(data)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
