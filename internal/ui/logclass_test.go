package ui

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		line string
		want logKind
	}{
		{"[12:00:00] [Server thread/INFO] [minecraft/DedicatedServer]: <Steve> hello there", kindChat},
		{"[12:00:00] [Server thread/INFO]: [Not Secure] <Alex> hi", kindChat},
		{"[12:00:00] [Server thread/INFO]: [Server] restarting in 5", kindChat},
		{"[12:00:00] [Server thread/INFO]: Steve joined the game", kindChat},
		{"[12:00:00] [Server thread/INFO]: Alex left the game", kindChat},
		{"[12:00:00] [Server thread/INFO]: Steve has made the advancement [Stone Age]", kindChat},

		{"[12:00:00] [Server thread/WARN] [minecraft/Foo]: something odd", kindNotable},
		{"[12:00:00] [Server thread/ERROR]: boom", kindNotable},
		{`[12:00:00] [Server thread/INFO]: Done (12.345s)! For help, type "help"`, kindNotable},
		{"[12:00:00] [Server thread/INFO]: Stopping server", kindNotable},

		{"[12:00:00] [Server thread/WARN]: Can't keep up! Is the server overloaded? Running 2100ms behind", kindNoise},
		{"[12:00:00] [Server thread/INFO]: Preparing spawn area: 24%", kindNoise},
		{"[12:00:00] [Server thread/INFO]: Preparing start region for dimension minecraft:overworld", kindNoise},
		{"[12:00:00] [Server thread/DEBUG]: chunk saved", kindNoise},
		{"[12:00:00] [Server thread/INFO]: Steve moved too quickly! -12.0,0.0,3.4", kindNoise},

		{"[12:00:00] [Server thread/INFO]: Some ordinary informational line", kindNormal},
	}
	for _, c := range cases {
		if got := classify(c.line); got != c.want {
			t.Errorf("classify(%q) = %d, want %d", c.line, got, c.want)
		}
	}
}
