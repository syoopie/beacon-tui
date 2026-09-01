package ui

import "testing"

func TestFormatConsoleLine(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"vanilla info",
			"[12:00:00] [Server thread/INFO]: Starting minecraft server version 1.20.1",
			"12:00:00  Starting minecraft server version 1.20.1",
		},
		{
			"vanilla warn keeps its tag",
			"[12:00:44] [Server thread/WARN]: Can't keep up! Running 2551ms behind",
			"12:00:44  WARN  Can't keep up! Running 2551ms behind",
		},
		{
			"forge line drops date and logger bracket",
			"[01Sep2026 16:03:56.842] [main/INFO] [cpw.mods.modlauncher.Launcher/MODLAUNCHER]: ModLauncher running",
			"16:03:56  ModLauncher running",
		},
		{
			"forge warn",
			"[01Sep2026 16:03:57.972] [main/WARN] [CrashAssistantJarInJarHelper/]: Crash Assistant is client only",
			"16:03:57  WARN  Crash Assistant is client only",
		},
		{
			"error tag",
			"[12:01:02] [Server thread/ERROR]: Encountered an unexpected exception",
			"12:01:02  ERROR  Encountered an unexpected exception",
		},
		{
			"chat keeps the message verbatim",
			"[12:05:00] [Server thread/INFO]: <yoopieee> hello there",
			"12:05:00  <yoopieee> hello there",
		},
		{
			"message with its own brackets survives",
			"[12:05:00] [Server thread/INFO]: [CHAT] something",
			"12:05:00  [CHAT] something",
		},
		{
			"stack frame is left alone",
			"    at java.lang.Thread.run(Thread.java:840)",
			"    at java.lang.Thread.run(Thread.java:840)",
		},
		{
			"non-log line is left alone",
			"    - fabric_api 0.92.6+1.11.15+1.20.1",
			"    - fabric_api 0.92.6+1.11.15+1.20.1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatConsoleLine(tc.in); got != tc.want {
				t.Fatalf("formatConsoleLine(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}
