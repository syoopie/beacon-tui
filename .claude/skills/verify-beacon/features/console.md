# Console

The full-screen log view for one server: the server's log on the left, a rail of
players and process stats on the right. This is where every rendering bug in
Beacon has so far turned up, because it is the only screen whose content is
arbitrary text from outside the program.

## Sub-features

- **Where the key hints live.** The console keys sit next to what they act on,
  not in one long bar. Server-level keys (`s` start or stop, `a` settings, `esc`
  back, `K` force-kill once a stop hangs) are the top command bar. `tab` is by
  the two tabs. `f` leads the hint row over the log (`logKeysView`), which also
  carries `↑↓ scroll`, `end latest`, `ctrl+f find`; that row stands in for the
  old rule, so it costs no height, and falls back to a plain rule while the
  input is open. `t` and `/` are a hint row just above the status line, where
  the input opens, shown only while the server is running. There is no `q`:
  `esc` is the only way out of every screen (`ctrl+c` still hard-quits).
- **Server log tab** and **Chat tab**, switched with `tab` (hinted right after
  the Chat tab, dropped only when the log pane is too narrow for it and the
  view word both).
- **Compact line format.** The console rewrites each log line for display
  (`formatConsoleLine`, `internal/ui/logfmt.go`): the full `[date time] [thread/
  LEVEL] [logger]:` prefix collapses to a bare `HH:MM:SS`, and a level tag is
  kept only for `WARN` and above. The line on disk is untouched; a line that is
  not a server-log line (a stack frame, a mod banner) is shown as-is. Search and
  the noise filter both work on this compact text.
- **Noise filter**, toggled with `f`. Filtered hides chatter; full shows
  everything with warnings and errors in the warning colour and noise dimmed.
  The tab bar's right word names the current view (`full log` / `important
  only`); the hint row below leads with `f` named for the other one, so
  `full log` up top and `f important only` below reads as "press f to switch".
- **Empty log**: a server with no log lines yet shows `No log yet. Press s to
  start the server.` centred in the pane (a waiting line instead while it
  runs).
- **The header** (`logHeaderView`) is name, status, port, launch method. Facts
  that do not fit drop whole from the right; name and status always stay.
- **Search**, opened with `ctrl+f`, narrowing the active tab as you type. `enter`
  keeps the filter, `esc` clears it.
- **Scrolling** with the arrow keys, `logScrollStep` lines per press. `end` (or
  `G`) jumps to the newest line, `home` (or `g`) to the oldest. The view opens
  at the newest line; when lines arrive while it is scrolled off the tail, a
  centred `↓ new lines below   end jump down` nudge shows on its own row under
  the log (`newLinesRow`, a blank row otherwise, so the log height never
  shifts). Scrolling up alone does not show it; `model.newBelow` is set by an
  arriving line and cleared whenever the view is back at the bottom.
- **The rail**: Details then Players while the server is stopped; while it
  runs, Resources first, then Players, then Details, so a short terminal clips
  the fixed details rather than the live numbers. Resources holds uptime, tick speed (`tps 20.0   2.7 ms/tick`) over a graph of
  tick time against the 50 ms budget, then CPU and memory, each a value line
  over a graph (`ntcharts` sparkline, newest sample on the right), and the
  share of host RAM. CPU is the rate between two `ps` samples, drawn against
  one core or its peak; memory is drawn against the JVM's `-Xmx`, read from
  the command line or an `@argfile` such as `user_jvm_args.txt` (`heap 8.0G`),
  or against its peak when there is none. Tick speed rides on the RCON player
  poll (`rcon.Client.tick`): the first poll tries `neoforge tps`, `forge tps`,
  `tick query`, `tps` and the client keeps the first that parses, so the TPS
  line is missing with RCON off or on a server none of them answers. Graphs
  are two rows, drop to one and then to none when Resources and Players would
  outgrow the body (`railView`); Details below them is clipped, and a heading
  left with none of its rows is dropped. A tick time
  far under 50 ms draws as a blank graph row, not a missing one. It only
  appears above 64 inner columns. The port line (and the header's) adds
  `starting` while a live session has not opened its port, `stopping` once a
  shutdown has closed it, `ready` when it accepts connections
  (`portHealthLabel`). Until the port opens, the Players section reads
  `starting up…` instead of an RCON error.
- **The input**, only open while the server is running, sends the line to the
  server's stdin on `enter` and then closes, the same as `esc` (the sent line
  shows on the status line). A slash line goes as typed; a plain line is chat
  and goes as `say <text>` (`serverLine`), since the console runs anything it
  reads as a command. It works like Minecraft's own chat
  box: `t` opens it empty, `/` opens it already holding a slash. A line that starts with `/` is **command mode** (`model.commandMode`) -
  the completion panel shows and `↑` / `↓` cycle it; any other line is plain and
  `↑` / `↓` walk the per-server command history (`internal/mccmd`, persisted to
  `state/history/<id>.txt`). Typing or deleting the leading slash flips between
  the two and resizes the log above.
- **Command completion**, a fixed 6-row panel above the input in command mode
  only (`internal/ui/complete.go`, `completionPanelH`). One status line (the
  Brigadier-style usage hint, e.g. `<targets> <item> [<count>]`, or a fix-it
  note when the tree is off) over a windowed suggestion list; `↑` / `↓` and
  `tab` / `shift+tab` cycle the highlight into the token being typed. The engine
  is a bundled vanilla command tree picked by the spec's `[commands] mc_version`;
  with no version set the panel shows a "could not detect this server's
  Minecraft version" note that points at `a` → Launch settings.
- **Modded commands over RCON.** When the server is running with RCON on, Beacon
  reads its `/help` once per session (`rcon.Help`, multi-packet) and folds the
  listed commands into the tree one level deep (`mccmd.HelpSource`, priority
  below bundled so the vanilla grammar still wins for shared commands). So on a
  Forge pack `/ftb…` completes to `ftbquests` and `/forge ` lists
  `tps|track|entity|…`. The fetch is on the `tickMsg` cadence, so it lands a
  second or two after the console opens, not instantly. Paper's plugin-grouped
  `/help` format is not parsed yet.
- **Online player names.** An argument slot that takes a player
  (`minecraft:entity`, `minecraft:game_profile`, `minecraft:score_holder`) is
  completed with the names of whoever is online, from the same RCON player poll
  that feeds the rail (`Engine.SetPlayers`). So with two people on, `/kill `
  lists their names above the `<targets>` usage hint, and `/kill No` narrows to
  the one that matches. Empty until the first poll returns and while nobody is
  online.

## How to get to it (user POV)

From the list: `→` or `enter` on a server opens its console. `esc` goes back
(`left` is a no-op here on purpose). With a log search active, the first `esc`
clears the search and the second leaves.

## Driving it with drive.py

```sh
key:right snap:console                    # -> the console, opened at the tail in "full log"
'key:up*8' snap:scrolled                  # scroll up; no nudge until a line arrives
key:end key:f snap:important              # f switches to "important only"
key:tab snap:chat                         # chat tab
'key:ctrl+f' key:y key:o key:o snap:search  # search for "yoo"
key:esc key:esc snap:back
```

Command completion needs a running server (start a throwaway tmux session
`beacon-<id>` running `sleep`, for an id only the fixture has, see
[lifecycle.md](lifecycle.md)) and a `[commands] mc_version` line in the
fixture spec. With `mc_version = ""` the panel shows the "could not detect"
note instead:

```sh
key:right 'key:/'               # open in command mode, holding "/"
key:g key:a key:m snap:typed              # "/gam" -> gamemode|gamerule
key:down key:down snap:cycle              # down cycles the token in place
'key:bs*8' key:g key:i key:v key:e key:space snap:hint   # "/give " -> usage hint
key:enter                                 # sends "/give"; the line is added to history
key:t key:up snap:recall                  # t opens a plain line; up recalls the last command
```

`t` instead of `/` opens the input empty for a chat line: no completion panel,
and `↑` / `↓` are history from the first keystroke.

Modded-command completion needs a *real* server with RCON on (a `sleep` tmux
session will not answer `/help`). Against the BMC4 pack
(`~/MinecraftServer/BMC4_ServerPack_v61`, Forge, RCON 25575), started so its
tmux session is `beacon-bmc4_serverpack_v61`:

```sh
key:right 'key:/' wait:3         # command mode; the tick fetches /help over RCON
key:f key:t key:b snap:modded              # "/ftb" -> ftbfiltersystem|ftblibrary|ftbquests|ftbteams
'key:bs*3' key:f key:o key:r key:g key:e key:space snap:forgesub  # "/forge " -> tps|track|entity|…
```

Player-name completion needs someone actually connected to the server, which a
scripted drive cannot arrange. `TestConsoleCompletionSuggestsOnlinePlayers` in
`internal/ui` drives a real `rconMsg` roster through the model instead; on a
server with players on, `'key:/' key:k key:i key:l key:l key:space` shows their
names above the `[<targets>]` hint.

`check_console.py` is the automated version for the rail: several widths, both
filter modes, twenty scroll steps each, asserting the rail border holds one
column throughout.

## Gotchas

- **The log is not trusted input.** Minecraft indents stack frames with a real
  tab and can emit escape sequences. `ansi.StringWidth` counts a tab as one
  column and a terminal draws it as up to eight, so an unsanitized line is drawn
  wider than it was measured and shoves the rail sideways. `sanitize` in
  `internal/ui/follow.go` expands tabs and strips escapes on the way in; every
  width calculation downstream depends on it.
- **Do not wrap twice.** `logBody` wraps with `ansi.Wrap`; `renderLog` must hand
  that straight to the viewport. Passing it through a lipgloss `Width` style
  re-wraps the already-wrapped rows and leaves short ragged fragments.
- The `f` and `tab` keys both jump the view back to the bottom, so a scroll
  position does not survive them. Scroll in `full log`: the important-only view
  of a short log can fit on screen with nothing to scroll.
- `bs` counts matter in command mode. Deleting the leading `/` drops the line
  into plain mode, so `/gamerule` (9 characters) takes `bs*8` to leave `/`.
- Fixture logs need a `logs/` line count in the thousands to scroll far enough
  to reach interesting content; `BMC4_ServerPack_v61/logs/latest.log` has ~6000.
- The rail says "RCON is off" unless the spec has `[rcon] enabled = true`. Edit
  the fixture's `servers/*.toml` to exercise the player list.
- **Beacon writes into the server log it shows.** Every RCON connection logs
  `Thread RCON Client /127.0.0.1 started` / `shutting down` on the server, and
  the `/help` fetch on a pack with FTB Essentials logs two `ERROR Error getting
  permission value for node ftbessentials.rtp...` lines the first time the
  input opens in a Beacon session (`helpFetchCmd` waits for `m.console`).
  These are Beacon's own traffic, not the fixture's, and they land wherever
  the drive is looking. Another Beacon open on the same machine
  (`pgrep -fl beacon`) adds its own.
- The Resources graphs need samples: the process graphs fill one column per
  3 s poll, tick time one per 10 s RCON poll. `wait:25` after opening the
  console shows a few columns of each. At 90x24 the graphs are one row and
  Details is clipped off entirely; at 123x40 everything fits with two-row
  graphs.
