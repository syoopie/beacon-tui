# Config editor

The modal that edits a server's `server.properties`. It surfaces a curated set
of keys grouped into sections, writes only the keys that changed, and leaves the
rest of the file (comments, ordering, keys it does not show) untouched.
`internal/ui/config.go` holds it; `internal/mcprops` does the line-preserving
write and `internal/lifecycle` `EditProperties` takes the lock and mirrors the
port and RCON block into the spec.

## Sub-features

- **The field list**: five sections (General, Gameplay, Access, World, RCON)
  drawn as bold headers, ~25 fields under them. Three kinds: free text, an enum
  cycled with `←`/`→`, and a bool cycled the same way.
- **Scrolling**: the fields sit in a viewport that keeps the cursor row on
  screen. The viewport takes whatever height is left once the title, subtitle
  and hint bar are measured, so the modal fits inside `m.bodyH` down to a
  72x20 terminal. Its floor is 3 visible rows.
- **Validation**: each field carries its own rule. Ports are 1..65535,
  max-players is at least 1, spawn-protection is at least 0, view-distance and
  simulation-distance are 3..32. A bad value keeps the editor open with the
  message in the status line and writes nothing.
- **The RCON group**: turning `enable-rcon` on writes `rcon.port` and
  `rcon.password` with it, even when they still hold the shown values, and the
  save is refused if the password is empty.
- **Save** (`enter`) writes and returns to the console. **Cancel** (`esc`)
  steps back to the settings overlay with nothing written.

## How to get to it (user POV)

From the list: `→` to focus the detail column, `enter` on **Open console**, `a`
for the settings overlay, `enter` on **Edit config** (it is the first row).
`esc` backs out one level at a time: editor, then settings, then console.

## Driving it with drive.py

```sh
key:right key:enter key:a key:enter snap:cfg_top   # open the editor
'key:down*9' snap:cfg_mid                          # scroll: sections move with the fields
'key:down*24' snap:cfg_bot                         # last field (rcon.password) and hint bar in view
```

Edit and save a value. `←`/`→` cycle an enum or bool; a text field takes typed
characters after the cursor lands on it:

```sh
key:right key:enter key:a key:enter \
  'key:down*5' key:right \                         # cursor to Hardcore, flip false -> true
  key:enter wait:1 snap:saved                      # status line reads "config saved"
```

Then read the server's `server.properties` and check the one key changed and
the rest of the file is intact.

Run at least `--rows 20` and `--rows 24`. The modal sizes itself to the body
height, so a clipping regression only shows on a short terminal.

## Gotchas

- **Model tests cannot see this screen.** The viewport, the scroll offset and
  the fit-to-`m.bodyH` math are all invisible to `tea.Model.View()`. A short
  terminal that clips the hint bar or the bottom border passes every Go test.
- **Sandbox the whole scan tree, not just the spec.** Beacon re-scans every
  path in `config.toml` `scan_roots` on each tick and rewrites the matching
  spec's `dir` back to where it found the folder. Editing `dir` in
  `servers/<id>.toml` alone is undone within a second. Point `scan_roots` at a
  throwaway directory that holds a copy of the server folder, and put the
  spec's `dir` inside it.
- **Save writes the real `server.properties`** in the server's `dir`. Use the
  sandboxed copy above, or a drive will edit the user's server.
- **Section headers are render only.** The cursor steps field to field and
  never lands on a header; the code bridges the two by the header's rendered
  line index, so a row that wraps instead of clipping would throw the scroll
  math off. Every row is clipped to width for that reason.
- `rcon.password` renders as bullets in the list and asterisks when it is the
  focused field. The value is still in `cf.values`; it is only the display that
  is masked.
- The editor reads `server.properties` fresh when it opens. A value changed on
  disk after the modal is up will not show until it is reopened.
