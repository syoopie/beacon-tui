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
- **Search**, opened with `ctrl+f`, narrowing the active tab as you type. `enter`
  keeps the filter, `esc` clears it.
- **Scrolling** with the arrow keys, `logScrollStep` lines per press. `end` (or
  `G`) jumps to the newest line, `home` (or `g`) to the oldest. The view opens
  at the newest line; when it has been scrolled off the tail while new lines
  keep arriving, a centred `↓ new lines below   end jump down` nudge shows on
  its own row under the log (`newLinesRow`, a blank row when at the bottom, so
  the log height never shifts).
- **The rail**: player list over RCON, then memory and CPU from `ps`. It only
  appears above 64 inner columns.
- **The input**, only open while the server is running, sends whatever is typed
  straight to the server's stdin on `enter` and then closes, the same as `esc`
  (the sent line shows on the status line). It works like Minecraft's own chat
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
key:right snap:console                    # -> the console, opened at the tail
key:f snap:full                           # toggle the noise filter
'key:up*8' snap:scrolled                  # into the stack trace
key:tab snap:chat                         # chat tab
'key:ctrl+f' key:y key:o key:o snap:search  # search for "yoo"
key:esc key:esc snap:back
```

Command completion needs a running server (start a throwaway tmux session
`beacon-<id>` running `sleep`) and a `[commands] mc_version` line in the
fixture spec:

```sh
key:right 'key:/'               # open in command mode, holding "/"
key:g key:a key:m snap:typed              # "/gam" -> gamemode|gamerule
key:down key:down snap:cycle              # down cycles the token in place
'key:bs*9' key:g key:i key:v key:e key:space snap:hint   # "/give " -> usage hint
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
  position does not survive them.
- Fixture logs need a `logs/` line count in the thousands to scroll far enough
  to reach interesting content; `BMC4_ServerPack_v61/logs/latest.log` has ~6000.
- The rail says "RCON is off" unless the spec has `[rcon] enabled = true`. Edit
  the fixture's `servers/*.toml` to exercise the player list.
