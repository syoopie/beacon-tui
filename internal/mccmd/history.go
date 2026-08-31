package mccmd

import (
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// History is a server's recent console lines, oldest first, for up/down recall
// in the console input. It is deliberately not a [VocabularySource]: a past line
// is recall, not grammar, and folding one into the command tree would only
// pollute it.
type History struct {
	mu    sync.Mutex
	lines []string
	max   int
}

// DefaultHistoryMax is the cap [NewHistory] uses when given a non-positive max.
const DefaultHistoryMax = 200

// NewHistory returns an empty history holding at most max lines.
func NewHistory(max int) *History {
	if max <= 0 {
		max = DefaultHistoryMax
	}
	return &History{max: max, lines: make([]string, 0, max)}
}

// LoadHistory reads a history file written by [History.Save]. A missing file is
// not an error: it returns an empty history.
func LoadHistory(path string, max int) (*History, error) {
	h := NewHistory(max)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return h, nil
	}
	if err != nil {
		return h, err
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	for sc.Scan() {
		if line := strings.TrimRight(sc.Text(), "\r\n"); line != "" {
			h.Add(line)
		}
	}
	return h, sc.Err()
}

// Add records a line as the most recent entry. A line identical to the current
// most recent entry is dropped, so holding Enter does not fill the history with
// one command. Blank lines are ignored.
func (h *History) Add(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if n := len(h.lines); n > 0 && h.lines[n-1] == line {
		return
	}
	h.lines = append(h.lines, line)
	if len(h.lines) > h.max {
		h.lines = append(h.lines[:0], h.lines[len(h.lines)-h.max:]...)
	}
}

// All returns a copy of the history, oldest first.
func (h *History) All() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.lines...)
}

// Len reports how many lines the history holds.
func (h *History) Len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.lines)
}

// Save writes the history to path, one line per entry, newest last, creating
// the parent directory if needed. The file is 0600 to match the rest of
// beacon's state.
func (h *History) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var buf bytes.Buffer
	for _, line := range h.All() {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}
