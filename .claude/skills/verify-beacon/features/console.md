# Console

The full-screen log view for one server: the server's log on the left, a rail of
players and process stats on the right. This is where every rendering bug in
Beacon has so far turned up, because it is the only screen whose content is
arbitrary text from outside the program.

## Sub-features

- **Server log tab** and **Chat tab**, switched with `tab`.
- **Noise filter**, toggled with `f`. Filtered hides chatter; full shows
  everything with warnings and errors in the warning colour and noise dimmed.
- **Search**, opened with `/`, narrowing the active tab as you type. `enter`
  keeps the filter, `esc` clears it.
- **Scrolling** with the arrow keys, three lines per press (`logScrollStep`).
  The view opens at the newest line.
- **The rail**: player list over RCON, then memory and CPU from `ps`. It only
  appears above 64 inner columns.
- **Sending a command** to a running server, opened with `c`.

## How to get to it (user POV)

From the list: `→` to focus the detail column, then `enter` on **Open console**.
`esc` goes back. With a search filter active, the first `esc` clears the filter
and the second leaves.

## Driving it with drive.py

```sh
key:right key:enter snap:console          # open at the tail
key:f snap:full                           # unfiltered
'key:up*8' snap:scrolled                  # into the stack trace
key:tab snap:chat                         # chat tab
'key:/' key:y key:o key:o snap:search     # search for "yoo"
key:esc key:esc snap:back
```

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
