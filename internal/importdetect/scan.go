// Package importdetect finds Minecraft server directories under the configured
// scan roots and turns them into specs.
package importdetect

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sunyupei/beacon-tui/internal/server"
)

// Candidate is a directory that looks like a Minecraft server, before IDs are
// assigned and collisions are resolved.
type Candidate struct {
	Dir    string    // absolute
	Base   server.ID // derived from the directory name, before any -2 suffix
	Start  string    // shell command to run inside Dir
	Script string    // start script relative to Dir, empty when a jar is launched directly
	Exec   server.ExecState
	Port   int
}

const defaultPort = 25565

var scriptNames = []string{"run.sh", "start.sh"}

var jarPatterns = []string{"server.jar", "paper*.jar", "fabric-server*.jar"}

// Scan inspects each configured root and its immediate subdirectories. It never
// recurses further: scan roots are configuration, not a search hint.
func Scan(roots []string) ([]Candidate, error) {
	if len(roots) == 0 {
		return nil, errors.New("scanning for servers: no scan root configured")
	}

	var cands []Candidate
	seen := make(map[string]bool)
	for _, root := range roots {
		dirs, err := scanTargets(root)
		if err != nil {
			return nil, err
		}
		for _, dir := range dirs {
			if seen[dir] {
				continue
			}
			seen[dir] = true
			if c, ok := inspectDir(dir); ok {
				cands = append(cands, c)
			}
		}
	}
	return cands, nil
}

// scanTargets returns the root itself followed by its immediate subdirectories.
// os.DirEntry.IsDir is false for a symlink, which is how a symlinked entry is
// kept from turning into a scan root of its own.
func scanTargets(root string) ([]string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolving scan root %s: %w", root, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("reading scan root %s: %w", abs, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("scan root %s: not a directory", abs)
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, fmt.Errorf("reading scan root %s: %w", abs, err)
	}

	dirs := []string{abs}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dirs = append(dirs, filepath.Join(abs, e.Name()))
	}
	return dirs, nil
}

func inspectDir(dir string) (Candidate, bool) {
	base, err := server.IDFromDir(dir)
	if err != nil {
		return Candidate{}, false
	}
	c := Candidate{Dir: dir, Base: base, Port: readPort(dir)}

	for _, name := range scriptNames {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		c.Script = name
		c.Start = "./" + name
		// An unreadable script cannot be proven to exec, and refusing to launch
		// one directory beats aborting the operator's whole import.
		check, err := InspectScript(path)
		if err != nil {
			c.Exec = server.ExecMissing
		} else {
			c.Exec = check.State
		}
		return c, true
	}

	if jar, ok := findJar(dir); ok {
		c.Start = "java -jar " + jar + " nogui"
		// beacon generates this command itself, so it always execs.
		c.Exec = server.ExecOK
		return c, true
	}
	return Candidate{}, false
}

// findJar matches against entry names rather than globbing the directory path,
// so a root containing a glob metacharacter still matches.
func findJar(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, pattern := range jarPatterns {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if ok, _ := filepath.Match(pattern, e.Name()); ok {
				return e.Name(), true
			}
		}
	}
	return "", false
}

// readPort is deliberately not a properties parser: phase 9's internal/mcprops
// owns the file, and import only needs a port to show the operator.
func readPort(dir string) int {
	path := filepath.Join(dir, "server.properties")
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultPort
	}
	for _, line := range strings.Split(string(data), "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "server-port=")
		if !ok {
			continue
		}
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return defaultPort
		}
		return port
	}
	return defaultPort
}
