package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A terminal expands a tab to the next tab stop and swallows escape sequences,
// while every width function in the layout counts a tab as one column and an
// escape as none. A log line carrying either is therefore drawn wider than the
// column it was measured for, which is what used to shove the console's side
// rail sideways on stack-trace rows.
func TestSanitizeMakesLineWidthMeasurable(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"stack frame tab", "\tat java.lang.Thread.run(Thread.java:840)", "    at java.lang.Thread.run(Thread.java:840)"},
		{"tab mid line", "a\tb", "a   b"},
		{"tab lands on the stop", "abcd\te", "abcd    e"},
		{"colour codes", "\x1b[32mDone\x1b[0m (31.4s)", "Done (31.4s)"},
		{"carriage return", "Preparing spawn area: 42%\r", "Preparing spawn area: 42%"},
		{"already clean", "[12:00:00] [Server thread/INFO]: hello", "[12:00:00] [Server thread/INFO]: hello"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitize(tc.in)
			if got != tc.want {
				t.Fatalf("sanitize(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if strings.ContainsFunc(got, func(r rune) bool { return r < 0x20 }) {
				t.Fatalf("control character survived: %q", got)
			}
		})
	}
}

// logBody hands the viewport lines that must each fit the log column, or the
// rail beside it moves. A tab counted as one column is the way that used to
// break, so the fixture is a real stack frame.
func TestLogBodyRowsFitTheColumn(t *testing.T) {
	m := &model{tail: &logFollower{}, logFull: true}
	m.vp.Width = 60
	m.tail.append([]string{
		"\tat net.minecraft.server.MinecraftServer.m_130011_(MinecraftServer.java:689) ~[server-1.20.1-20230612.114412-srg.jar%23729!/:?]",
		"\tat java.lang.Thread.run(Thread.java:840) ~[?:?]",
		"[12:00:00] [Server thread/INFO]: short",
	}, maxLogLines)

	for i, row := range strings.Split(m.logBody(), "\n") {
		if w := ansi.StringWidth(row); w > m.vp.Width {
			t.Errorf("row %d is %d columns wide, column is %d: %q", i, w, m.vp.Width, row)
		}
		if strings.ContainsRune(row, '\t') {
			t.Errorf("row %d still carries a tab: %q", i, row)
		}
	}
}
