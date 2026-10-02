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

Start from an empty config so the landing panel shows, and give the drive a
scan root:

```sh
--server ~/MinecraftServer \
  snap:landing key:a snap:picker key:right snap:inside key:enter snap:imported
```

To exercise the patch dialog, copy a pack to a scratch dir and rewrite its
`run.sh` so the java line has no `exec`, then import that copy. The status
line names the fix but not the key (see Gotchas); the actual path is the
server's own console, not the list:

```sh
key:right                                 # import already selects the new row; open its console
key:a snap:actions                        # actions overlay: "Fix start script" is the top row
key:enter snap:patch_dialog                # the diff/backup preview
key:y wait:0.5 snap:patched                # applies; status line reads "<id> patched (ok)"
```

## Gotchas

- `beacon <dir>` seeds a scan root and scans it on launch, so its servers are
  listed on boot.
- The picker's row count depends on the terminal height, so its snapshot is not
  stable across `--rows` values.
- The patch writes to the user's real folder. Copy the pack first.
- The post-import status line ("N need `exec` patching (select and press
  p)") names a `p` key that does not exist (`internal/ui/import.go`); the real
  path is the console's actions overlay, `a` → Fix start script. Product bug,
  not a driving mistake if the recipe above stalls waiting for a `p` press.
- Import writes `servers/<id>.toml` into the config dir. Use a per-run copy of
  the fixture, or the next drive starts from different state.
