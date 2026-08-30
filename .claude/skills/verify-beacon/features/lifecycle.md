# Start, stop, force kill, mark stopped

The actions in the detail column that move a server between states. Each one
runs under the host operation lock in `internal/lifecycle`, with tmux owning the
process.

**This is the one feature that touches real state.** A drive that starts a
server starts a JVM and a tmux session on the machine you are on. Decide that
deliberately, and tear it down.

## Sub-features

- **Start**: refuses when the start script does not hand off with `exec`, or
  when the port is already taken.
- **Stop**: types `stop` into the session and waits out `StopTimeout`.
- **Force kill**: offered once a stop has run long.
- **Mark stopped**: clears an unknown status the operator has checked by hand.

Which rows appear depends on the derived status, so the action list is itself
worth snapshotting per state.

## How to get to it (user POV)

Select the server, `→`, then `enter` on the row.

## Driving it with drive.py

Read-only half, safe anywhere:

```sh
key:right snap:actions-stopped         # which rows exist while stopped
```

The mutating half needs a throwaway fixture and a server you are willing to
run:

```sh
W=/tmp/beacon-lifecycle; rm -rf $W; cp -R /tmp/beacon-fixture $W
# ... drive with --config-dir $W/config --state-dir $W/state ...
tmux kill-session -t beacon-bmc4_serverpack_v61
```

A modpack server takes minutes to reach "Done", so `wait:` in tens of seconds,
and prefer watching the console log for the "Done (" line over guessing.

## Gotchas

- Never verify Start against the user's real config dir. Copy the fixture.
- `tmux kill-session` is the teardown; killing the JVM by name can hit an
  unrelated Minecraft server.
- The status shown is derived from tmux each tick, not from what the action
  returned, so assert on the screen after a `wait:`, not immediately.
- Port collision detection needs something actually listening on 25565 to
  exercise; `nc -l 25565` in another shell is enough.
