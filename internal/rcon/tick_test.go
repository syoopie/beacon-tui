package rcon

import "testing"

func TestTickParsers(t *testing.T) {
	cases := []struct {
		name  string
		parse func(string) (Tick, bool)
		in    string
		want  Tick
	}{
		{"neoforge", parseNeoForgeTPS,
			"Overworld: 20.000 TPS (0.808 ms/tick)\nThe Nether: 20.000 TPS (0.041 ms/tick)\nOverall: 20.000 TPS (2.065 ms/tick)\n",
			Tick{TPS: 20, MSPT: 2.065}},
		{"forge", parseForgeTPS,
			"Dim minecraft:overworld (overworld): Mean tick time: 61.200 ms. Mean TPS: 16.340\nOverall: Mean tick time: 62,500 ms. Mean TPS: 16,000",
			Tick{TPS: 16, MSPT: 62.5}},
		{"tick query at speed", parseTickQuery,
			"The game is running normally\nTarget tick rate: 20.0 per second.\nAverage time per tick: 2.1ms (Target: 50.0ms)\nPercentiles: P50: 2.3ms P95: 3.1ms P99: 3.9ms, sample: 100\n",
			Tick{TPS: 20, MSPT: 2.1}},
		{"tick query lagging", parseTickQuery,
			"Target tick rate: 20.0 per second.\nAverage time per tick: 80.0ms (Target: 50.0ms)",
			Tick{TPS: 12.5, MSPT: 80}},
		{"paper tps", parseBukkitTPS,
			"§6TPS from last 1m, 5m, 15m: §a*20.0, §a19.8, §a19.9",
			Tick{TPS: 20}},
	}
	for _, c := range cases {
		got, ok := c.parse(c.in)
		if !ok || got != c.want {
			t.Fatalf("%s: got %+v %v, want %+v", c.name, got, ok, c.want)
		}
	}
	for _, p := range []func(string) (Tick, bool){parseNeoForgeTPS, parseForgeTPS, parseTickQuery, parseBukkitTPS} {
		if _, ok := p("Unknown or incomplete command, see below for error\ntps<--[HERE]"); ok {
			t.Fatal("an unknown-command reply should not parse")
		}
	}
}
