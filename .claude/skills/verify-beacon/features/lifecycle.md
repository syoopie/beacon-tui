# Start, stop, force kill, mark stopped

The keys on the console that move a server between states. Each one runs under
the host operation lock in `internal/lifecycle`, with tmux owning the process.

**This is the one feature that touches real state.** A drive that starts a
server starts a JVM and a tmux session on the machine you are on. Decide that
deliberately, and tear it down.

## Sub-features

- **`s`** is the primary action, labelled for the derived status
  (`primaryAction`): `s start` while stopped, `s stop` while running, `s mark
  stopped` for a status Beacon has lost. It shows in the console command bar.
  While a server is starting or stopping `s` is disabled and drops out of the
  bar, so a second start or stop cannot be stacked on one in flight.
- **Start** refuses when the start script does not hand off with `exec`, when the
  server's Java setting points at a file that is not a runnable executable, when
  something already listens on the port, when the Minecraft EULA has not been
  accepted, or when no Java is set and no installed JDK is new enough for the
  server's Minecraft version (`<id> needs Java N or newer ...`)
  (`internal/lifecycle/lifecycle.go`); the reason lands on the status line.
  Another stopped server configured for the same port is named but does not
  block.
- **Java runtime** per server, set in Launch settings (`a` → Launch settings, the
  row under MC version, `←→` to cycle). The first choice, **Automatic**, stores
  an empty `java`: Start then runs the oldest installed JDK the spec's
  `mc_version` accepts (`javadetect.RequiredMajor` and `Pick`). A pick
  is stored as the spec's `java`, and `internal/tmux` prepends its directory to
  `PATH` for the launch, so a bare `java` in the command or a `run.sh` resolves
  to it. `internal/javadetect` finds the host's JDKs for the picker.
- **Stop** opens a confirm modal (`m.stop`, a centred `stopPrompt` dialog):
  `s` sets it, `y` types `stop` into the session and waits out `StopTimeout`,
  `esc` or `n` cancels with `stop cancelled` on the status line, and a stray key
  is ignored so the modal stays up.
- **Force kill** is **`K`**, and it only appears in the command bar once a stop
  has timed out (`m.timedOut[id]`, set by the `opDoneMsg` a slow stop returns).
- **Mark stopped** is what `s` does for an unknown status: it clears the status
  once the operator has checked by hand. Unknown now means the session is gone
  and something still holds the port. A session gone with its port free derives
  as stopped, with a `stopped on its own` notice, and `s` starts it again; a
  session gone after a recorded Stopping is plain stopped.

## How to get to it (user POV)

Select the server, `→` to open its console, then `s` (or `K` after a stop
hangs). There is no menu row for these any more.

## Driving it with drive.py

Read-only half, safe anywhere. Open the console and read the command bar to see
which action `s` offers for the current status:

```sh
wait:1.5 key:right snap:console        # command bar shows "s start" for a stopped server
```

The mutating half needs a throwaway fixture and a server you are willing to
run:

```sh
W=/tmp/beacon-lifecycle; rm -rf $W; cp -R /tmp/beacon-fixture $W
# drive with --config-dir $W/config --state-dir $W/state:
#   key:right key:s wait:30 snap:starting ...   (s on a stopped server)
#   key:right key:s key:y wait:20 snap:stopping (s then y on a running one)
tmux kill-session -t beacon-bmc4_serverpack_v61
```

Start can be driven without a JVM that stays up. A sandbox pack (see
[adding-a-server.md](adding-a-server.md)) whose `server.jar` is an empty file,
with its EULA accepted, starts for real and dies at once with `Error: Invalid
or corrupt jarfile server.jar`. The session is gone by the next reconcile, so
nothing needs tearing down, and the `stopped on its own` notice quotes that
line. The status line reads `<id> starting…` until a reconcile settles it,
then `<id> exited while starting; ...` here, or `<id> is ready for players`
once a real server's port opens (`followLaunch`).

A modpack server takes minutes to reach "Done", so `wait:` in tens of seconds,
and prefer watching the console log for the "Done (" line over guessing.

## Gotchas

- Never verify Start against the user's real config dir. Copy the fixture.
- `s` on a running server only opens the confirm modal; the stop does not run
  until `y`. A drive that sends `key:s` and asserts immediately sees the modal
  (`Stop <id>?`), not a stopped server. To verify the modal without a real JVM,
  fake the session with `tmux new-session -d -s beacon-<id> 'sleep 900'` so
  reconcile derives running, then drive `key:right key:s snap:` and `key:esc`.
  tmux is machine-wide, so any Beacon the user has open sees that session too.
  Fake it for an id that only the fixture has (an imported sandbox pack), not
  one of the user's real servers.
- To see both lost-session notices, set the fixture spec's `[state] last_known
  = "running"` with no session: a free port gives `stopped on its own`, a held
  port gives unknown and `s mark stopped`.
- `tmux kill-session` is the teardown; killing the JVM by name can hit an
  unrelated Minecraft server.
- The status shown is derived from tmux each tick, not from what the action
  returned, so assert on the screen after a `wait:`, not immediately.
- Port collision detection only fires on a live listener. `nc -l <port>` is
  not enough: Beacon's port probe connects, and `nc` exits after that first
  connection. Hold the port with a listener that keeps accepting:

  ```sh
  python3 -c "import socket; s=socket.socket(); s.bind(('',25565)); s.listen(); [s.accept()[0].close() for _ in iter(int, 1)]" &
  ```

  Two stopped specs sharing a port is allowed, so a drive that wants the
  refusal has to bind the port for real.
- `K` does nothing until `m.timedOut[id]` is set, which only happens after a
  real stop overruns `StopTimeout`. You cannot shortcut to the force-kill path.
