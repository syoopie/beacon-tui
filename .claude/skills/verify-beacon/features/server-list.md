# Server list

Beacon's home screen (`screenList`). Every configured server in one full-width
table with its derived status; an always-on search box above it; a centred
landing panel instead when nothing is configured. There is no detail column and
no per-server menu: `→` or `enter` opens the selected server's console, where
every action lives.

## Sub-features

- **Selection** with `↑` / `↓`. The selected server owns the notice banner, so
  it moves with the cursor.
- **Open the console** with `→` or `enter` on a server row.
- **The always-on filter**: the search box at the top is always focused, so any
  `[a-z0-9_-]` character (case-folded) types into it and the list narrows live;
  backspace edits. The box shows `N shown` while it has text. `esc` clears a
  non-empty filter, and quits Beacon when it is already empty. There is no `/`
  to enter filter mode; typing is the filter.
- **The add row** sits just above the list, shown only when there is a list and
  no active filter. `↑` from the first server steps onto it, `↓` steps back,
  `enter` opens the folder picker. (`a` opens the picker only on the empty
  landing panel; on a populated list `a` is just a filter character.)
- **Rescan** with `ctrl+r`: drops servers whose folder is gone (unless still
  running), then re-reads every configured scan root and imports anything new
  without the picker.
- **Columns** drop as the terminal narrows (`columnsFor`): below 55 columns the
  row is one loose line, then name+status, then +port, then +health-dot+detail.
- **Notice banner** above the table when the selected server needs attention
  (`noticeText`). A session that ended without Beacon stopping it gives one of
  two warnings, both quoting the last line of the captured log: with its port
  free it reads as **stopped** with `<id> stopped on its own. Its log ends: ...
  Open its console and press s to start it again.` (`crashedWarning`); with
  something still holding the port it is **unknown** with `Beacon did not stop
  <id>, but its session is gone ...` and points at `s` to mark it stopped
  (`vanishedWarning`). The other notices: a start script that does not `exec`
  java (or, when the folder has a Forge/NeoForge installer, `... press a,
  choose Launch settings and pick <installer option>`), and an unaccepted EULA
  (`... press a, then choose Accept the Minecraft EULA`). Every key is named for
  the screen it shows on (`pressIn`): `open its console and press a` on the
  list, `press a` on the console, which shows the same banner.
- **The empty state**: a centred landing panel when no server is configured,
  with its own command bar `a add server · esc quit`.

## How to get to it (user POV)

It is the screen Beacon opens on. `esc` from the console returns here.

## Driving it with drive.py

```sh
wait:1.5 snap:list                         # boot, let the first reconcile land
key:down snap:selected
key:b key:r snap:filtered                   # type part of an id to filter; no key:/ needed
key:esc snap:unfiltered                     # esc clears the filter
key:right snap:console                      # -> the selected server's console
```

The command bar at the top reads
`↑ up · ↓ down · → console · ctrl+r scan folders · esc quit`
(`esc clear search` while filtering, `enter add server` on the add row), and
wraps onto more rows when the terminal is narrow. There is no separate help
screen and no `q` or `?` binding.

Two servers make the filter and add-row behaviour visible. `drive.py` with no
`--config-dir` builds a fresh empty one and renders the landing panel; point
`--config-dir` at a throwaway dir holding two minimal `servers/*.toml` for the
populated case.

## Gotchas

- Status comes from a reconcile tick against tmux, so a freshly booted drive
  shows the last known status until the first tick lands. Give it `wait:1.5`
  before asserting on a status word.
- The notice banner changes the body height, and `relayout` runs on selection
  change for that reason. Snapshot a server with a warning and one without to
  check the body does not jump.
- The list takes the whole body width now; the console screen is the one with
  the rail. A narrow-terminal regression shows in the console, not here.
- Import runs only on launch with a folder argument (`beacon <dir>`), on
  `ctrl+r`, and after an add from the picker; never on a tick. It adds folders
  no spec claims yet and never rewrites an existing spec. A fixture whose
  `scan_roots` point at real directories imports from them on `ctrl+r`.
- Deleting a server's folder and pressing `ctrl+r` drops the server and also
  its scan root (the picker adds a server's own folder as one), so the status
  line reads `removed 1 server whose folder is gone` and `config.toml` no
  longer lists the folder (`Manager.PruneScanRoots`).
