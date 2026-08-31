package ui

import "regexp"

// logKind sorts a server log line into a tier. The tier drives two things: which
// lines the console keeps when it is filtered to what matters, and how the line
// is coloured. On a modded server the log level is almost useless as a signal,
// thousands of harmless lines are logged at WARN and ERROR, so the tiers come
// from curated patterns rather than the level.
type logKind int

const (
	kindNormal logKind = iota // plain informational line, no special treatment
	kindNoise                 // known spam: dimmed in the full log, dropped from the filtered one
	kindChat                  // a player chat message, shown on the Chat tab
	kindEvent                 // worth seeing: joins, leaves, lifecycle, saves, admin commands
	kindWarn                  // the server runs but something wants attention
	kindError                 // the server cannot run or has crashed
)

// important reports whether a line survives the console's important-only filter.
// Chat is deliberately not important on the Server tab, it has its own tab.
func (k logKind) important() bool {
	return k == kindEvent || k == kindWarn || k == kindError
}

// onChatTab reports whether the Chat tab shows this line. Chat messages plus the
// player-facing events (joins, advancements, deaths) that used to live only on
// that tab.
func (k logKind) onChatTab() bool {
	return k == kindChat || k == kindEvent
}

type classRule struct {
	kind logKind
	re   *regexp.Regexp
}

// classRules is matched top to bottom, first hit wins. The order matters: the
// catastrophe markers come first so an out-of-memory line beats the generic
// stack-frame rule below it; the spam denylist comes before the chat and event
// allowlists so a noisy logger never gets promoted. An unmatched line stays
// kindNormal, which is never wrong, only unhelpful.
var classRules = []classRule{
	// Breaking. The server cannot start or has died. These strings are stable
	// across Minecraft versions.
	{kindError, regexp.MustCompile(`Unable to locate a Java Runtime|for information on installing Java`)},
	{kindError, regexp.MustCompile(`FAILED TO BIND TO PORT|Perhaps a server is already running`)},
	{kindError, regexp.MustCompile(`Encountered an unexpected exception`)},
	{kindError, regexp.MustCompile(`]: A single server tick took \d|server will forcibly shutdown`)},
	{kindError, regexp.MustCompile(`This crash report has been saved to|Exception in server tick loop`)},
	{kindError, regexp.MustCompile(`]: Failed to start the minecraft server|can't proceed with server load`)},
	{kindError, regexp.MustCompile(`OutOfMemoryError`)},

	// Noise: the tail of an exception, and the exception header itself. A
	// breaking crash still shows its headline line above these; the trace is for
	// the full log.
	{kindNoise, regexp.MustCompile(`^\s*at [\w./$]+\(`)},
	{kindNoise, regexp.MustCompile(`^\s*\.\.\. \d+ more$`)},
	{kindNoise, regexp.MustCompile(`^(?:Caused by|Suppressed): `)},
	{kindNoise, regexp.MustCompile(`^[\w.$]+(?:Exception|Error)(?::|\s|$)`)},

	// Noise: loggers that spam by the thousand on a modded server, plus the
	// lines the console has always treated as noise.
	{kindNoise, regexp.MustCompile(`/(?:DEBUG|TRACE)\]`)},
	{kindNoise, regexp.MustCompile(`RuntimeDistCleaner/DISTXFORM|\[mixin/\]: Error loading class`)},
	{kindNoise, regexp.MustCompile(`\[co\.re\.RecipeEssentials/\]|\[co\.st\.StructureEssentials/\]`)},
	{kindNoise, regexp.MustCompile(`Radium Class Analysis|\[or\.be\.wo\.to\.ut\.Logger/\]: \[bclib\]`)},
	{kindNoise, regexp.MustCompile(`\[minecraft/(?:RecipeManager|TagLoader|SimpleJsonResourceReloadListener|ServerAdvancementManager)\]`)},
	{kindNoise, regexp.MustCompile(`Couldn't parse element loot_tables:|has been registered twice`)},
	{kindNoise, regexp.MustCompile(`]: Thread RCON Client `)},
	{kindNoise, regexp.MustCompile(`moved (?:too quickly!|wrongly!)`)},
	{kindNoise, regexp.MustCompile(`]: Preparing (?:spawn area:|start region for)`)},
	{kindNoise, regexp.MustCompile(`]: Time elapsed:`)},
	{kindNoise, regexp.MustCompile(`]: Loaded \d+ (?:advancements|recipes)`)},
	{kindNoise, regexp.MustCompile(`Mismatch in destroy block pos`)},

	// Chat: what a player typed.
	{kindChat, regexp.MustCompile(`]: (?:\[Not Secure\] )?<[^<>]+> `)},
	{kindChat, regexp.MustCompile(`]: \[(?:Server|Not Secure)\] `)},
	{kindChat, regexp.MustCompile(`]: \* \S`)},
	{kindChat, regexp.MustCompile(` (?:was slain by|was shot by|was killed by|was blown up by|was fireballed by|drowned|blew up|hit the ground too hard|fell (?:from a high place|off|out of)|tried to swim in lava|went up in flames|burned to death|was pricked to death|walked into (?:a cactus|the danger zone)|was impaled|starved to death|suffocated in a wall|didn't want to live|withered away)\b`)},

	// Event: something an operator or player wants to see.
	{kindEvent, regexp.MustCompile(` (?:joined|left) the game\b`)},
	{kindEvent, regexp.MustCompile(` has (?:made the advancement|completed the challenge|reached the goal) `)},
	{kindEvent, regexp.MustCompile(`]: \S+ lost connection:|] logged in with entity id`)},
	{kindEvent, regexp.MustCompile(`]: Done \(|]: Starting minecraft server`)},
	{kindEvent, regexp.MustCompile(`]: Stopping the server|]: Stopping server\b`)},
	{kindEvent, regexp.MustCompile(`]: Preparing level `)},
	{kindEvent, regexp.MustCompile(`/RconThread\]: RCON running|]: Thread RCON Listener started`)},
	{kindEvent, regexp.MustCompile(`]: \S+ issued server command:`)},
	{kindEvent, regexp.MustCompile(`]: (?:Opped|De-opped|Made \S+ a server operator|Kicked|Banned|Unbanned) |the (?:whitelist|banlist)\b`)},
	{kindEvent, regexp.MustCompile(`\[minecraft/MinecraftServer\]: Sav(?:ing|ed) the game|]: Automatic saving is now (?:disabled|enabled)`)},

	// Attention: the server runs, but degraded or misconfigured.
	{kindWarn, regexp.MustCompile(`]: Can't keep up!|]: Running \d+ms or \d+ ticks behind`)},
	{kindWarn, regexp.MustCompile(`]: (?:Skipping|Failed to load) EULA|]: You need to agree to the EULA`)},
	{kindWarn, regexp.MustCompile(`(?:party|permission) system "[^"]+" isn't registered!`)},
}

func classify(line string) logKind {
	for _, r := range classRules {
		if r.re.MatchString(line) {
			return r.kind
		}
	}
	return kindNormal
}
