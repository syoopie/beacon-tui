# Adding a server

How a folder on disk becomes a server Beacon can run: the folder picker, the
scan in `internal/importdetect`, and the offer to patch a start script that does
not hand off to Java with `exec`.

## Sub-features

- **The picker**, a bubbles filepicker. `→` opens a folder, `←` goes up,
  `enter` adds the highlighted folder if it holds a server and opens it
  otherwise, `s` adds the folder shown in the header, `esc` cancels.
- **Detection**: the scan reads `server.properties` for the port and the RCON
  block, and finds a start script, a NeoForge or Forge installer jar, or a
  `server.jar`. An installer becomes the first launch option, a `sh -c` line
  that installs on first start and then `exec`s Java.
- **The patch dialog**: shown when the chosen script starts Java as a child
  instead of `exec`ing it. It previews the one-line diff, backs the original up
  to `<script>.bak`, and applies on `y`.
- **Rescan** (`ctrl+r`) picks up folders added under a configured scan root
  without going through the picker.

## How to get to it (user POV)

From the empty landing panel, `a`. From a populated list, step `↑` onto the
**Add a server** row and press `enter` (on a populated list `a` is a filter
character, not the add key). `ctrl+r` rescans without the picker from either.

## Driving it with drive.py

The picker always opens at `$HOME` (`openPicker`), and `drive.py` passes its
own environment to the child, so set `HOME` to a sandbox that holds copied
packs. Start from an empty config dir so the landing panel shows; do not pass
`--server`, which scans on launch and skips the landing panel.

```sh
SB=/tmp/beacon-sb; mkdir -p $SB/home/packs $SB/config $SB/state
# copy a pack into $SB/home/packs/<name>: server.properties, eula.txt, the jar
HOME=$SB/home drive.py --config-dir $SB/config --state-dir $SB/state \
  snap:landing key:a snap:picker key:right snap:inside key:enter wait:1 snap:imported
```

`key:right` opens `packs`, `enter` on a folder that holds a server adds it.
From a populated list the entry is `key:up` (repeat until the add row is
highlighted) then `key:enter`.

To exercise the patch dialog, copy a pack to a scratch dir and rewrite its
`run.sh` so the java line has no `exec`, then import that copy. The status
line reads `1 need \`exec\` patching (open its console, then a for Fix start
script)`. The add selects the new server, so `→` opens it:

```sh
key:right                                 # open the just-added server's console
key:a snap:actions                        # settings overlay: "Fix start script" is the top row
key:enter snap:patch_dialog                # the diff/backup preview
key:y wait:0.5 snap:patched                # applies; status line reads "<id> patched (ok)"
```

## Gotchas

- `beacon <dir>` seeds a scan root and scans it on launch, so its servers are
  listed on boot.
- The picker's row count depends on the terminal height, so its snapshot is not
  stable across `--rows` values.
- The patch writes to the user's real folder. Copy the pack first.
- After an add the cursor moves onto the server found in the picked folder,
  off the add row (`opDoneMsg.focus`, `model.focusID`).
- In the settings overlay "Fix start script" is the top row unless the EULA is
  also unaccepted, in which case "Accept the Minecraft EULA" sits above it.
- Import writes `servers/<id>.toml` into the config dir. Use a per-run copy of
  the fixture, or the next drive starts from different state.
