package ui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/syoopie/beacon-tui/internal/logtail"
)

// tabStop is how many columns a tab in a log line becomes. Minecraft indents
// every stack-trace frame with a real tab.
const tabStop = 4

// sanitize makes a log line safe to measure. A terminal expands a tab to the
// next tab stop and acts on escape sequences, but every width function here
// counts them as one column or none, so a line carrying either is drawn wider
// than Beacon thinks and overruns the column it was laid out for. Stack traces
// are full of tabs, which is why the console's side rail used to jog sideways
// on exactly the rows that had one.
//
// The server runs on a tmux PTY, so its console reader (JLine) thinks a human is
// at the keyboard and reprints its "> " prompt in front of many log lines,
// wrapped in an erase-line escape. Strip the escape, then the prompt, so the
// line starts where the timestamp does.
func sanitize(line string) string {
	line = ansi.Strip(line)
	line = strings.TrimLeft(line, "\r")
	for strings.HasPrefix(line, "> ") {
		line = line[2:]
	}
	if strings.ContainsRune(line, '\t') {
		var b strings.Builder
		col := 0
		for _, r := range line {
			if r == '\t' {
				n := tabStop - col%tabStop
				b.WriteString(strings.Repeat(" ", n))
				col += n
				continue
			}
			b.WriteRune(r)
			col += ansi.StringWidth(string(r))
		}
		line = b.String()
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, line)
}

// logEntry is one log line reduced to what the console needs: the compact text
// it renders and searches, and the tier it was sorted into. Both are computed
// once on the way in. Classification sees the full sanitized line, since several
// rules key off the thread and logger brackets that the compact form drops.
type logEntry struct {
	display string
	kind    logKind
}

// logFollower pairs a file tailer with the classified lines it has yielded so
// far, so the console can redraw either tab from a bounded buffer.
type logFollower struct {
	t       *logtail.Tailer
	entries []logEntry
}

func newFollower(path string) *logFollower {
	return &logFollower{t: logtail.NewTailer(path)}
}

func (f *logFollower) read() ([]string, error) {
	return f.t.Read()
}

// append classifies and stores each line, trimming the buffer to the most
// recent limit entries.
func (f *logFollower) append(lines []string, limit int) {
	for _, l := range lines {
		l = sanitize(l)
		f.entries = append(f.entries, logEntry{display: formatConsoleLine(l), kind: classify(l)})
	}
	if len(f.entries) > limit {
		f.entries = append(f.entries[:0:0], f.entries[len(f.entries)-limit:]...)
	}
}
