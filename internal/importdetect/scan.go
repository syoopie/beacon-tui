// Package importdetect finds Minecraft server directories under the configured
// scan roots and turns them into specs.
package importdetect

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/syoopie/beacon-tui/internal/mcprops"
	"github.com/syoopie/beacon-tui/internal/server"
)

// Candidate is a directory that looks like a Minecraft server, before IDs are
// assigned and collisions are resolved.
type Candidate struct {
	Dir       string    // absolute
	Base      server.ID // derived from the directory name, before any -2 suffix
	Start     string    // shell command to run inside Dir
	Script    string    // start script relative to Dir, empty when a jar is launched directly
	Exec      server.ExecState
	Port      int
	RCON      server.RCON
	MCVersion string // best-effort, "" when undetected
	Loader    string // best-effort, "" when undetected
}

var scriptNames = []string{"run.sh", "start.sh"}

var jarPatterns = []string{"server.jar", "paper*.jar", "fabric-server*.jar"}

// Scan inspects each configured root. If the root is itself a server directory
// (the common case: the operator picked the folder their server lives in), that
// is the one server the root contributes. Otherwise beacon looks one level down
// at the root's immediate subdirectories, so pointing it at a folder that holds
// several servers also works. It never recurses further, so a server's own
// plugins or mods folder is not mistaken for another server.
func Scan(roots []string) ([]Candidate, error) {
	if len(roots) == 0 {
		return nil, errors.New("scanning for servers: no scan root configured")
	}

	var cands []Candidate
	seen := make(map[string]bool)
	add := func(dir string) {
		if seen[dir] {
			return
		}
		seen[dir] = true
		if c, ok := inspectDir(dir); ok {
			cands = append(cands, c)
		}
	}

	for _, root := range roots {
		abs, err := scanRoot(root)
		if err != nil {
			return nil, err
		}
		if _, ok := inspectDir(abs); ok {
			add(abs)
			continue
		}
		children, err := os.ReadDir(abs)
		if err != nil {
			return nil, fmt.Errorf("reading scan root %s: %w", abs, err)
		}
		for _, e := range children {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			add(filepath.Join(abs, e.Name()))
		}
	}
	return cands, nil
}

func scanRoot(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolving scan root %s: %w", root, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("reading scan root %s: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("scan root %s: not a directory", abs)
	}
	return abs, nil
}

func inspectDir(dir string) (Candidate, bool) {
	base, err := server.IDFromDir(dir)
	if err != nil {
		return Candidate{}, false
	}
	opts := LaunchOptions(dir)
	if len(opts) == 0 {
		return Candidate{}, false
	}
	// Import takes the first option, the same priority order the operator sees
	// in the launch-settings modal, where they can switch to another. nogui
	// stops the server opening its own Swing console window; Beacon is the
	// console.
	o := opts[0]
	props, err := mcprops.LoadProperties(dir)
	if err != nil {
		props = mcprops.Empty()
	}
	mcVersion, loader := Identify(dir)
	return Candidate{
		Dir: dir, Base: base, Port: props.Port(),
		Script: o.Script, Exec: o.Exec, Start: o.Command("nogui"),
		RCON:      props.RCON(),
		MCVersion: mcVersion, Loader: loader,
	}, true
}

// LaunchOption is one way to start the server in a directory: a start script or
// a runnable jar. A pack often ships both run.sh and start.sh, so a directory
// can offer several.
type LaunchOption struct {
	Label  string           // "run.sh", "start.sh", or the jar's filename
	Script string           // script name relative to the directory; empty for a jar
	Base   string           // command without trailing arguments: "./run.sh" or "java -jar server.jar"
	Exec   server.ExecState // script: the inspected state; jar: always ExecOK
}

// Command joins the option's base command with the given extra arguments.
func (o LaunchOption) Command(args string) string {
	args = strings.TrimSpace(args)
	if args == "" {
		return o.Base
	}
	return o.Base + " " + args
}

// LaunchOptions lists every launch method in dir, scripts before jars, in the
// order import uses. An unreadable script is reported as ExecMissing rather than
// dropped, so refusing to launch it is a later, visible decision.
func LaunchOptions(dir string) []LaunchOption {
	var opts []LaunchOption
	for _, name := range scriptNames {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		exec := server.ExecMissing
		if check, err := InspectScript(path); err == nil {
			exec = check.State
		}
		opts = append(opts, LaunchOption{Label: name, Script: name, Base: "./" + name, Exec: exec})
	}
	for _, jar := range jarsIn(dir) {
		// beacon generates this command itself, so it always execs.
		opts = append(opts, LaunchOption{Label: jar, Base: "java -jar " + jar, Exec: server.ExecOK})
	}
	return opts
}

// jarsIn matches against entry names rather than globbing the directory path, so
// a root containing a glob metacharacter still matches.
func jarsIn(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var jars []string
	seen := map[string]bool{}
	for _, pattern := range jarPatterns {
		for _, e := range entries {
			if e.IsDir() || seen[e.Name()] {
				continue
			}
			if ok, _ := filepath.Match(pattern, e.Name()); ok {
				jars = append(jars, e.Name())
				seen[e.Name()] = true
			}
		}
	}
	return jars
}
