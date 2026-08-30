// Package mcprops reads and edits a Minecraft server's own config files:
// server.properties and eula.txt. Both are the same key=value format. Editing
// rewrites only the lines that change, so comments, blank lines, ordering, and
// keys Beacon does not surface all survive a round trip.
package mcprops

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/syoopie/beacon-tui/internal/config"
)

// Properties is a parsed key=value file that remembers its original layout.
type Properties struct {
	path  string
	lines []string       // every line of the file, verbatim
	index map[string]int // key -> line in lines, for keys that are present
}

// LoadProperties reads serverDir/server.properties. A missing file is not an
// error: it yields an empty Properties that Save will create.
func LoadProperties(serverDir string) (*Properties, error) {
	return load(filepath.Join(serverDir, "server.properties"))
}

func load(path string) (*Properties, error) {
	p := &Properties{path: path, index: map[string]int{}}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		p.lines = append(p.lines, line)
		if k, ok := keyOf(line); ok {
			p.index[k] = len(p.lines) - 1
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return p, nil
}

// keyOf returns the property key a line defines, or ok=false for a blank line,
// a comment (# or !), or a line with no '='.
func keyOf(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
		return "", false
	}
	k, _, ok := strings.Cut(trimmed, "=")
	if !ok {
		return "", false
	}
	return strings.TrimSpace(k), true
}

// Get returns the value for key and whether it was present.
func (p *Properties) Get(key string) (string, bool) {
	i, ok := p.index[key]
	if !ok {
		return "", false
	}
	_, v, _ := strings.Cut(p.lines[i], "=")
	return strings.TrimSpace(v), true
}

// GetOr returns the value for key, or def when it is absent.
func (p *Properties) GetOr(key, def string) string {
	if v, ok := p.Get(key); ok {
		return v
	}
	return def
}

// Set writes key=value, rewriting the key's existing line or appending a new
// one. The value is written verbatim; Minecraft reads these values raw.
func (p *Properties) Set(key, value string) {
	line := key + "=" + value
	if i, ok := p.index[key]; ok {
		p.lines[i] = line
		return
	}
	p.lines = append(p.lines, line)
	p.index[key] = len(p.lines) - 1
}

// Render returns the file's bytes with a trailing newline.
func (p *Properties) Render() []byte {
	return []byte(strings.Join(p.lines, "\n") + "\n")
}

// Save writes the file back atomically, creating it if it did not exist.
func (p *Properties) Save() error {
	return config.WriteFileAtomic(p.path, p.Render(), 0o644)
}

// EULAAccepted reports whether serverDir/eula.txt says eula=true. A missing file
// counts as not accepted, which is how Minecraft ships it.
func EULAAccepted(serverDir string) (bool, error) {
	p, err := load(filepath.Join(serverDir, "eula.txt"))
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(p.GetOr("eula", "false")), "true"), nil
}

// AcceptEULA writes eula=true into serverDir/eula.txt, keeping any comment lines
// Minecraft wrote (the link to the agreement).
func AcceptEULA(serverDir string) error {
	path := filepath.Join(serverDir, "eula.txt")
	p, err := load(path)
	if err != nil {
		return err
	}
	p.Set("eula", "true")
	return p.Save()
}
