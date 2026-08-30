package ui

import "github.com/sunyupei/beacon-tui/internal/tmux"

// logFollower pairs a file tailer with the lines it has yielded so far, so the
// viewport can be redrawn from a bounded buffer.
type logFollower struct {
	t     *tmux.Tailer
	lines []string
}

func newFollower(path string) *logFollower {
	return &logFollower{t: tmux.NewTailer(path)}
}

func (f *logFollower) read() ([]string, error) {
	return f.t.Read()
}

// append adds lines and trims the buffer to the most recent limit entries.
func (f *logFollower) append(lines []string, limit int) []string {
	f.lines = append(f.lines, lines...)
	if len(f.lines) > limit {
		f.lines = append(f.lines[:0:0], f.lines[len(f.lines)-limit:]...)
	}
	return f.lines
}
