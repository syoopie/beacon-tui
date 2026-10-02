package rcon

import (
	"regexp"
	"strconv"
	"strings"
)

// Tick is how fast a server is running its game loop.
type Tick struct {
	TPS  float64 // ticks per second achieved; 20 is full speed
	MSPT float64 // mean milliseconds per tick, 0 when the server does not report it
}

// tickSources are the commands that report tick speed, in the order Poll tries
// them. Loader commands come first: they measure TPS, where vanilla's "tick
// query" only gives the tick time it is derived from. Paper answers "tick
// query" too; "tps" is for Spigot and Paper builds older than 1.20.3.
var tickSources = []struct {
	cmd   string
	parse func(string) (Tick, bool)
}{
	{"neoforge tps", parseNeoForgeTPS},
	{"forge tps", parseForgeTPS},
	{"tick query", parseTickQuery},
	{"tps", parseBukkitTPS},
}

// num matches a decimal in either notation a server's locale may print.
const num = `(\d+(?:[.,]\d+)?)`

var (
	neoForgeRE  = regexp.MustCompile(`Overall:\s*` + num + ` TPS \(` + num + ` ms/tick\)`)
	forgeRE     = regexp.MustCompile(`Overall:\s*Mean tick time:\s*` + num + ` ms\. Mean TPS:\s*` + num)
	tickRateRE  = regexp.MustCompile(`Target tick rate:\s*` + num)
	tickTimeRE  = regexp.MustCompile(`Average time per tick:\s*` + num + `\s*ms`)
	bukkitTPSRE = regexp.MustCompile(`TPS from last [^:]*:\s*\*?` + num)
)

func parseNum(s string) float64 {
	f, _ := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64)
	return f
}

// parseNeoForgeTPS reads the overall line of "neoforge tps":
// "Overall: 20.000 TPS (2.065 ms/tick)".
func parseNeoForgeTPS(out string) (Tick, bool) {
	m := neoForgeRE.FindStringSubmatch(out)
	if m == nil {
		return Tick{}, false
	}
	return Tick{TPS: parseNum(m[1]), MSPT: parseNum(m[2])}, true
}

// parseForgeTPS reads the overall line of "forge tps":
// "Overall: Mean tick time: 2.065 ms. Mean TPS: 20.000".
func parseForgeTPS(out string) (Tick, bool) {
	m := forgeRE.FindStringSubmatch(out)
	if m == nil {
		return Tick{}, false
	}
	return Tick{TPS: parseNum(m[2]), MSPT: parseNum(m[1])}, true
}

// parseTickQuery reads vanilla's "tick query". It reports the target rate and
// the mean tick time; a server keeps its target until a tick outgrows its share
// of a second, and runs slower in proportion after that.
func parseTickQuery(out string) (Tick, bool) {
	rate, ms := tickRateRE.FindStringSubmatch(out), tickTimeRE.FindStringSubmatch(out)
	if rate == nil || ms == nil {
		return Tick{}, false
	}
	target, mspt := parseNum(rate[1]), parseNum(ms[1])
	tps := target
	if mspt > 0 {
		tps = min(target, 1000/mspt)
	}
	return Tick{TPS: tps, MSPT: mspt}, true
}

// parseBukkitTPS reads the one-minute figure of Spigot's and Paper's "tps":
// "TPS from last 1m, 5m, 15m: *20.0, 20.0, 20.0". The star marks a value
// over 20.
func parseBukkitTPS(out string) (Tick, bool) {
	m := bukkitTPSRE.FindStringSubmatch(stripCodes(out))
	if m == nil {
		return Tick{}, false
	}
	return Tick{TPS: parseNum(m[1])}, true
}
