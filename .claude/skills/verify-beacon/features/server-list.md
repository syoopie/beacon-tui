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
- **Rescan** with `ctrl+r`: re-reads every configured scan root and imports
  anything new without the picker.
- **Columns** drop as the terminal narrows (`columnsFor`): below 55 columns the
  row is one loose line, then name+status, then +port, then +health-dot+detail.
- **Notice banner** above the table when the selected server needs attention: an
  unknown status with a warning, a start script that does not `exec` java, or an
  unaccepted EULA.
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
- `ctrl+r` re-scans every path in `config.toml` `scan_roots`. A fixture whose
  scan roots point at real directories will re-import from them on every tick.
