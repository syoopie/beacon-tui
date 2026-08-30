# Server list and detail column

Beacon's home screen. Every configured server on the left with its derived
status, the selected server's details and actions on the right, a divider
between. The two columns share one body; only the focus changes between
`screenList` and `screenMenu`.

## Sub-features

- **Selection** with `↑` / `↓`, which also moves the detail column and the
  notice banner.
- **Focus** the actions with `→` or `enter`, back with `←` or `esc`. Unfocused
  action rows are dimmed and carry a `→ act on this server` hint.
- **Filter** the list with `/`.
- **Notice banner** above the body when the selected server needs attention: an
  unknown status with a warning, or a start script that does not `exec` java.
- **The empty state**, a centred landing panel when no server is configured.
- `a` add a server, `r` refresh, `?` the full key grid, `q` quit.

## How to get to it (user POV)

It is the screen Beacon opens on. `esc` from anywhere else returns here.

## Driving it with drive.py

```sh
snap:list                                  # boot straight into it
key:down key:down snap:selected
key:right snap:focused                     # actions focused
key:left snap:unfocused
'key:/' key:b key:m key:c snap:filtered    # filter to bmc4
key:esc snap:unfiltered
key:? snap:helpgrid
```

An empty config dir (omit `--config-dir` and `drive.py` makes a fresh one)
renders the landing panel instead.

## Gotchas

- The list column is clamped to 24-34 columns, so below about 60 columns the
  detail column gets very narrow rather than the list shrinking.
- The notice banner changes the body height, and `relayout` runs on selection
  change for exactly that reason. Snapshot a server with a warning and one
  without to check the body does not jump.
- Status comes from a reconcile tick against tmux, so a freshly booted drive
  shows the last known status until the first tick lands. Give it a `wait:1.5`
  before asserting on a status word.
