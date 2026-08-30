# Adding a server

How a folder on disk becomes a server Beacon can run: the folder picker, the
scan in `internal/importdetect`, and the offer to patch a start script that does
not hand off to Java with `exec`.

## Sub-features

- **The picker**, opened with `a`, a bubbles filepicker. `→` opens a folder,
  `←` goes up, `enter` chooses the current one, `esc` cancels.
- **Detection**: the scan reads `server.properties` for the port and the RCON
  block, and finds a start script or a `server.jar`.
- **The patch dialog**: shown when the chosen script starts Java as a child
  instead of `exec`ing it. It previews the one-line diff, backs the original up
  to `<script>.bak`, and applies on `y`.
- **Rescan** (`r`) picks up folders added under a configured scan root without
  going through the picker.

## How to get to it (user POV)

`a` from the list, or from the landing panel when no server exists yet.

## Driving it with drive.py

Start from an empty config so the landing panel shows, and give the drive a
scan root:

```sh
--server ~/MinecraftServer \
  snap:landing key:a snap:picker key:right snap:inside key:enter snap:imported
```

To exercise the patch dialog, copy a pack to a scratch dir and rewrite its
`run.sh` so the java line has no `exec`, then import that copy.

## Gotchas

- `beacon <dir>` only seeds a scan root; nothing is imported until the user
  presses `r` or walks the picker. A drive that expects a server to appear on
  boot will see the landing panel instead.
- The picker's row count depends on the terminal height, so its snapshot is not
  stable across `--rows` values.
- The patch writes to the user's real folder. Copy the pack first.
- Import writes `servers/<id>.toml` into the config dir. Use a per-run copy of
  the fixture, or the next drive starts from different state.
