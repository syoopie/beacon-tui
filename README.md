# Beacon

Run your Minecraft servers from one clean terminal screen. Start them, stop them,
watch their logs, all without hunting for the right `screen` session or leaving a
server half-dead.

[![CI](https://github.com/syoopie/beacon-tui/actions/workflows/ci.yml/badge.svg)](https://github.com/syoopie/beacon-tui/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/syoopie/beacon-tui?sort=semver)](https://github.com/syoopie/beacon-tui/releases)
[![License](https://img.shields.io/github/license/syoopie/beacon-tui)](LICENSE)

```text
  Beacon                                         ↑ v0.2.0 available
  ↑ up • ↓ down • → actions • / filter • a add server • ? more keys • q quit

  Servers                    │  survival
                             │  ● running   port 25565
  ▸ ● survival      running  │  via run.sh
    ○ creative      stopped  │
    ◆ skyblock      unknown  │    Open console
                             │    Stop
                             │    Edit config
                             │    Launch settings
                             │
                             │  →  act on this server

  ready
```

The panel on the right follows the highlighted server: its status, its port, the
script or jar it starts with, and the actions that apply to it right now. Press
`→` to step into that panel, `↑` `↓` to pick an action, `enter` to run it, `←` to
step back to the list. "Open console" fills the screen with the server log. The
bar under the title always shows the keys that work right now; `?` expands it.

## Why Beacon

- **Your servers stay up when you leave.** Beacon is just a control panel. Close
  it, log out, come back tomorrow. The servers keep running.
- **Nothing gets started twice.** Open Beacon in three windows if you like. It
  will not launch a second copy of a server or let two people fight over one.
- **Stops are safe.** Beacon asks the server to save and shut down, waits, and
  only offers a hard kill if it hangs. It never yanks the plug on its own.
- **It finds your servers for you.** Point it at a server's folder and it works
  out how to launch it, whether that is a `run.sh`, a Paper jar, or a Fabric jar.
- **One file, no fuss.** Download one binary. The only thing you need installed
  is `tmux`.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/syoopie/beacon-tui/main/install.sh | bash
```

You also need `tmux`:

- macOS: `brew install tmux`
- Debian / Ubuntu: `sudo apt-get install -y tmux`

That is it. Beacon runs on macOS and Linux. Windows is not supported.

## Getting started

Run it:

```sh
beacon
```

The first run opens on a welcome screen because you have no servers yet.

1. Press `a` and pick the folder your server lives in. Beacon browses your
   folders, so you do not need to type a path.
2. Your server shows up in the list. Move the highlight with `↑` and `↓`; the
   panel on the right updates to match.
3. Press `→` to step into that panel. It holds the actions that make sense right
   now (start, stop, open console, edit config, launch settings). Pick one with
   `↑` `↓` and `enter`; `←` steps back to the list.
4. "Open console" is a full screen: the server log, scrollable. Press `c` there
   to type a command straight to the server.
5. "Edit config" opens the common `server.properties` settings (port, MOTD,
   difficulty, max players, RCON). Beacon writes only the keys you change.
6. Have many servers? Press `/` to filter the list by name.

Have several servers, each in its own folder? Add each one with `a`. Or point
Beacon at a folder that contains all of them and it picks up every server inside.

The panel's header shows which script or jar Beacon starts each server with
(`via run.sh`). A pack that ships more than one launcher, such as a `run.sh` and
a `start.sh`, defaults to `run.sh`; the "Launch settings" action switches it or
sets the arguments passed to it.

### Keys

On the list:

| Key     | What it does                         |
| ------- | ------------------------------------ |
| `↑` `↓` | move the highlight                   |
| `→`     | step into the server's actions       |
| `/`     | filter the list by name              |
| `a`     | add a server (pick its folder)       |
| `i`     | re-scan your folders for new servers |
| `u`     | show the command to update Beacon    |
| `r`     | refresh now                          |
| `?`     | show every key                       |
| `q`     | quit (your servers keep running)     |

In the action panel: `↑` `↓` move, `enter` runs the row, `←` or `esc` goes back
to the list. The rows depend on status: start, stop, open console, edit config,
mark stopped, force-kill (only after a stop hangs), accept the Minecraft EULA
(until you have), fix start script, launch settings.

In the config editor: `↑` `↓` move between fields, `←` `→` change a choice or
toggle, type into a text field, `enter` saves, `esc` cancels. Turning RCON on
writes its port and password too, and mirrors them into Beacon so the player
list works without a re-import.

In the console screen: `↑` `↓` scroll, `c` types a command to the server, `←` or
`esc` goes back. `tab` switches between the server log and a Chat view that shows
only player activity. On the server log, `f` toggles between a filtered view that
drops routine noise and the full log, where the noise is dimmed and the lines
worth reading stand out. `/` searches the current view, narrowing it to matching
lines until you clear it with `esc`.

On wide terminals a rail on the right shows who is online and the server
process's memory and CPU. The player list needs RCON, which the "Edit config"
action turns on for you. The memory and CPU figures come from `ps` and need
nothing configured.

Next to the port, Beacon shows whether that port is accepting connections.
`starting` means the process is up but has not opened its port yet, the normal
state for the first half minute of a boot. `ready` means players can join. A
server that sits on `starting` while its log has gone quiet is wedged.

Beacon will not start a server whose `eula.txt` does not say `eula=true`. The
"Accept the Minecraft EULA" action writes it once you agree to
<https://aka.ms/MinecraftEULA>.

## Questions

**Where does Beacon keep things?** Settings live in `config.toml` under your
config folder (`~/Library/Application Support/beacon` on macOS, `~/.config/beacon`
on Linux). Logs live under `~/.local/state/beacon/logs`. You rarely need to touch
either.

**My server flips straight to `unknown` when I start it.** It started and then
exited on its own. The log panel on the right shows why. The usual cause is Java
not being on your `PATH`; most modpacks need Java 17 or 21. Install it (macOS:
`brew install openjdk@17`, then follow the caveats `brew` prints so `java` is
found), then try again.

**Can I use it on a remote box?** Yes. Beacon is a local program with no network
of its own. However you already get a terminal on that machine (SSH, Tailscale,
sitting in front of it) is how you use Beacon.

**What is that `unknown` status?** Beacon knows a server was running, but its
session has vanished. It will not guess that the server is safely off, because
guessing wrong is how you get two servers fighting over one port. Check the box,
then run the "Mark stopped" action.

**Does it keep itself updated?** It tells you when a new version is out and shows
the command to run. Updating is re-running the install line above.

## Development

Requires Go 1.24+ and `tmux`.

```sh
git clone https://github.com/syoopie/beacon-tui
cd beacon-tui
make check        # gofmt, go vet, go test
make test-unix    # also runs the tmux integration tests
make run          # go run ./cmd/beacon
```

The tmux integration tests carry a `//go:build unix` tag and skip when `tmux` is
not on `PATH`. `make lint` runs golangci-lint if it is installed; CI always does.

### How it works

- **tmux owns process lifetime and stdin.** Beacon launches each server through a
  shell that redirects output to a log file and `exec`s the command, so the pane
  process is the JVM itself. Sessions are named `beacon-<id>`.
- **Logs are plain files**, tailed from disk. The tailer reopens a log that is
  truncated or rotated.
- **Disk is the source of truth.** `config.toml` and `servers/<id>.toml` hold
  everything that matters. Every Beacon process reloads them each tick. Memory is
  a cache that goes stale until the next reconcile.
- **A host lock serializes every mutating operation** (start, stop, force-kill,
  import, script patch, config write) across all Beacon processes. It is a
  PID-bearing lockfile with stale-holder recovery. Reads never take it.
- **Reconcile** compares recorded state against tmux on startup and on a ticker,
  and derives the status shown in the list.

### Package layout

| Package                 | Responsibility                                    |
| ----------------------- | ------------------------------------------------- |
| `internal/config`       | dirs, TOML parse, atomic write                    |
| `internal/server`       | IDs, spec, the status state machine               |
| `internal/importdetect` | scan folders, detect scripts and jars, exec patch |
| `internal/supervisor`   | the `Supervisor` port                             |
| `internal/tmux`         | the tmux adapter, the only package that knows tmux |
| `internal/logtail`      | append-only log file follower, reopens on truncate |
| `internal/reconcile`    | derive status from tmux, port collision and live port health checks |
| `internal/oplock`       | the host operation lock                           |
| `internal/lifecycle`    | start, stop, force-kill, config writes under the lock |
| `internal/mcprops`      | line-preserving editor for server.properties and eula.txt |
| `internal/rcon`         | poll a running server for its player list         |
| `internal/procstat`     | read a process's memory and CPU from `ps`         |
| `internal/selfupdate`   | the startup release check                         |
| `internal/ui`           | the Bubble Tea front end                          |

### Cutting a release

Releases are built by a manually triggered GitHub Actions workflow, not on every
push. In the **Actions** tab, run **Release** and pass a version such as `v0.1.0`.
It cross-compiles `darwin` and `linux` for `amd64` and `arm64`, publishes the
binaries with checksums, and tags the commit.

## License

[MIT](LICENSE)
