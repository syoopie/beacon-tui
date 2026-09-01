package ui

import (
	"regexp"
	"strings"
)

// logLineRE splits a Minecraft server log line into its clock time, the
// bracketed thread (which carries the level after a slash), any number of
// further brackets (the logger name on a Forge line), and the message. It
// matches the vanilla shape
//
//	[12:00:00] [Server thread/INFO]: msg
//
// and the Forge shape, which prefixes the date and appends a logger bracket:
//
//	[01Sep2026 16:03:56.842] [main/INFO] [cpw.mods.modlauncher.Launcher/]: msg
var logLineRE = regexp.MustCompile(
	`^\[(?:\d{1,2}[A-Za-z]{3}\d{4} )?(\d{2}:\d{2}:\d{2})(?:\.\d+)?\] \[([^\]]*)\](?: \[[^\]]*\])*: (.*)$`)

// quietLevels are the log levels the compact console view does not tag: INFO is
// the default and DEBUG/TRACE are already dimmed as noise, so a tag would just
// add clutter. WARN and above keep theirs.
var quietLevels = map[string]bool{"": true, "INFO": true, "DEBUG": true, "TRACE": true}

// formatConsoleLine rewrites one sanitized log line for the console: the full
// timestamp becomes a bare "HH:MM:SS", the thread and logger brackets are
// dropped, and a level tag survives only for WARN and above. A line that is not
// a server-log line (a stack frame, a mod's own multi-line banner, a blank) is
// returned unchanged, so the raw text still shows where it matters.
func formatConsoleLine(raw string) string {
	m := logLineRE.FindStringSubmatch(raw)
	if m == nil {
		return raw
	}
	clock, thread, msg := m[1], m[2], m[3]

	level := thread
	if i := strings.LastIndexByte(thread, '/'); i >= 0 {
		level = thread[i+1:]
	}
	level = strings.ToUpper(strings.TrimSpace(level))

	if quietLevels[level] {
		return clock + "  " + msg
	}
	return clock + "  " + level + "  " + msg
}
