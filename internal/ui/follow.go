package ui

import "github.com/syoopie/beacon-tui/internal/tmux"

// logEntry is one log line plus the tier it was sorted into when it arrived.
type logEntry struct {
	raw  string
	kind logKind
}

// logFollower pairs a file tailer with the classified lines it has yielded so
// far, so the console can redraw either tab from a bounded buffer.
type logFollower struct {
	t       *tmux.Tailer
	entries []logEntry
}

func newFollower(path string) *logFollower {
	return &logFollower{t: tmux.NewTailer(path)}
}

func (f *logFollower) read() ([]string, error) {
	return f.t.Read()
}

// append classifies and stores each line, trimming the buffer to the most
// recent limit entries.
func (f *logFollower) append(lines []string, limit int) {
	for _, l := range lines {
		f.entries = append(f.entries, logEntry{raw: l, kind: classify(l)})
	}
	if len(f.entries) > limit {
		f.entries = append(f.entries[:0:0], f.entries[len(f.entries)-limit:]...)
	}
}
