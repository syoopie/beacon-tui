package ui

import (
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		line string
		want logKind
	}{
		// Chat: what a player typed.
		{"[12:00:00] [Server thread/INFO] [minecraft/MinecraftServer]: <Steve> hello there", kindChat},
		{"[12:00:00] [Server thread/INFO]: [Not Secure] <Alex> hi", kindChat},
		{"[12:00:00] [Server thread/INFO]: [Server] restarting in 5", kindChat},
		{"[12:00:00] [Server thread/INFO]: * Steve waves", kindChat},
		{"[12:00:00] [Server thread/INFO]: Steve was slain by Zombie", kindChat},
		{"[12:00:00] [Server thread/INFO]: Alex fell from a high place", kindChat},

		// Event: joins, leaves, lifecycle, admin actions.
		{"[12:00:00] [Server thread/INFO] [minecraft/MinecraftServer]: Steve joined the game", kindEvent},
		{"[12:00:00] [Server thread/INFO] [minecraft/MinecraftServer]: Alex left the game", kindEvent},
		{"[12:00:00] [Server thread/INFO]: Steve has made the advancement [Stone Age]", kindEvent},
		{"[12:00:00] [Server thread/INFO] [minecraft/ServerGamePacketListenerImpl]: Steve lost connection: Disconnected", kindEvent},
		{"[12:00:00] [Server thread/INFO] [minecraft/PlayerList]: Steve[/192.0.2.5:52679] logged in with entity id 251 at (1, 2, 3)", kindEvent},
		{`[12:00:00] [Server thread/INFO] [minecraft/DedicatedServer]: Done (12.345s)! For help, type "help"`, kindEvent},
		{"[12:00:00] [Server thread/INFO] [minecraft/DedicatedServer]: Starting minecraft server version 1.20.1", kindEvent},
		{"[12:00:00] [Server thread/INFO] [minecraft/MinecraftServer]: Stopping the server", kindEvent},
		{"[12:00:00] [Server thread/INFO] [minecraft/DedicatedServer]: Preparing level \"world\"", kindEvent},
		{"[12:00:00] [Server thread/INFO] [minecraft/RconThread]: RCON running on 0.0.0.0:25575", kindEvent},
		{"[12:00:00] [Server thread/INFO]: Steve issued server command: /gamemode creative", kindEvent},
		{"[12:00:00] [Server thread/INFO] [minecraft/MinecraftServer]: Saving the game (this may take a moment!)", kindEvent},

		// Attention: runs but degraded.
		{"[12:00:00] [Server thread/WARN]: Can't keep up! Is the server overloaded? Running 2100ms or 42 ticks behind", kindWarn},
		{"[12:00:00] [Server thread/WARN] [xa.pa.OpenPartiesAndClaims/]: The configured primary party system \"argonauts_guilds\" isn't registered!", kindWarn},
		{"[12:00:00] [Server thread/INFO] [minecraft/DedicatedServer]: You need to agree to the EULA in order to run the server", kindWarn},

		// Breaking.
		{"The operation couldn't be completed. Unable to locate a Java Runtime.", kindError},
		{"[12:00:00] [Server thread/WARN]: **** FAILED TO BIND TO PORT!", kindError},
		{"[12:00:00] [Server thread/WARN]: Perhaps a server is already running on that port?", kindError},
		{"[12:00:00] [Server thread/ERROR] [minecraft/MinecraftServer]: Encountered an unexpected exception", kindError},
		{"[12:00:00] [Server Watchdog/ERROR] [minecraft/MinecraftServer]: A single server tick took 60.00 seconds", kindError},
		{"[12:00:00] [Server thread/ERROR]: This crash report has been saved to: /srv/crash-reports/x.txt", kindError},
		{"java.lang.OutOfMemoryError: Java heap space", kindError},
		{"Caused by: java.lang.OutOfMemoryError: Metaspace", kindError},

		// Noise: modded-server spam and the tail of an exception.
		{"[12:00:00] [main/ERROR] [ne.mi.fm.lo.RuntimeDistCleaner/DISTXFORM]: Attempted to load class net/minecraft/client/Foo for invalid dist DEDICATED_SERVER", kindNoise},
		{"[12:00:00] [Render thread/WARN] [co.re.RecipeEssentials/]: whatever", kindNoise},
		{"[12:00:00] [worker/WARN] [Radium Class Analysis/]: scanning", kindNoise},
		{"[12:00:00] [main/ERROR] [minecraft/RecipeManager]: Parsing error loading recipe foo:bar", kindNoise},
		{"[12:00:00] [main/ERROR] [or.be.wo.to.ut.Logger/]: [bclib] ERROR building loot table", kindNoise},
		{"[12:00:00] [main/WARN] [mixin/]: Error loading class: betterdays/client/Foo", kindNoise},
		{"[12:00:00] [RCON Listener #1/INFO] [minecraft/GenericThread]: Thread RCON Client /127.0.0.1 started", kindNoise},
		{"[12:00:00] [Server thread/DEBUG]: chunk saved", kindNoise},
		{"[12:00:00] [Server thread/INFO]: Preparing spawn area: 24%", kindNoise},
		{"[12:00:00] [Server thread/INFO]: Preparing start region for dimension minecraft:overworld", kindNoise},
		{"[12:00:00] [Server thread/INFO]: Steve moved too quickly! -12.0,0.0,3.4", kindNoise},
		{"[12:00:00] [Server thread/INFO]: Time elapsed: 1523 ms", kindNoise},
		{"\tat net.minecraft.server.MinecraftServer.runServer(MinecraftServer.java:689)", kindNoise},
		{"... 12 more", kindNoise},
		{"java.lang.NullPointerException: Cannot invoke something", kindNoise},

		// Unmatched WARN and ERROR stay normal. Level is not a signal here.
		{"[12:00:00] [Server thread/WARN] [minecraft/Foo]: some unrecognised warning", kindNormal},
		{"[12:00:00] [Server thread/ERROR]: some unrecognised error", kindNormal},
		{"[12:00:00] [Server thread/INFO]: Some ordinary informational line", kindNormal},
	}
	for _, c := range cases {
		if got := classify(c.line); got != c.want {
			t.Errorf("classify(%q) = %d, want %d", c.line, got, c.want)
		}
	}
}

func TestImportantOnlyEmptyPlaceholder(t *testing.T) {
	m := &model{tail: &logFollower{}, logTab: tabServer, logImportantOnly: true}
	m.vp.Width = 80
	m.tail.append([]string{
		"[12:00:00] [Server thread/INFO]: an ordinary line",
		"[12:00:01] [Server thread/INFO]: Preparing spawn area: 5%",
	}, maxLogLines)

	body := m.logBody()
	if !strings.Contains(body, "no warnings or errors") {
		t.Fatalf("important only with nothing to show should render the placeholder, got:\n%s", body)
	}

	m.tail.append([]string{"[12:00:02] [Server thread/INFO]: Alex joined the game"}, maxLogLines)
	if strings.Contains(m.logBody(), "no warnings or errors") {
		t.Fatalf("placeholder should be gone once an important line arrives:\n%s", m.logBody())
	}
}

func TestLogKindPredicates(t *testing.T) {
	important := map[logKind]bool{kindEvent: true, kindWarn: true, kindError: true}
	chat := map[logKind]bool{kindChat: true, kindEvent: true}
	for _, k := range []logKind{kindNormal, kindNoise, kindChat, kindEvent, kindWarn, kindError} {
		if k.important() != important[k] {
			t.Errorf("kind %d important() = %v", k, k.important())
		}
		if k.onChatTab() != chat[k] {
			t.Errorf("kind %d onChatTab() = %v", k, k.onChatTab())
		}
	}
}
