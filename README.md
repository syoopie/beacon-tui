# Beacon

Run your Minecraft servers from one clean terminal screen. Start them, stop them,
watch their logs, all without hunting for the right `screen` session or leaving a
server half-dead.

[![CI](https://github.com/syoopie/beacon-tui/actions/workflows/ci.yml/badge.svg)](https://github.com/syoopie/beacon-tui/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/syoopie/beacon-tui?sort=semver)](https://github.com/syoopie/beacon-tui/releases)
[![License](https://img.shields.io/github/license/syoopie/beacon-tui)](LICENSE)

```text
  Beacon                                         ↑ v0.2.0 available
  ↑ up • ↓ down • x stop • c console • a add server • ? more keys • q quit

  Servers                  │  survival   ·   running   ·   port 25565
                           │  ──────────────────────────────────────────────────
  ▸ ● survival     running │  [12:31:02] [Server thread/INFO]: Done (6.4s)!
    ○ creative     stopped │  [12:31:02] [Server thread/INFO]: For help, type "help"
    ◆ skyblock     unknown │  [12:33:20] [Server thread/INFO]: Steve joined the game

  survival stopped
```

The bar under the title always shows the keys that do something right now. Press
`?` for the full list.

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
2. Your server shows up in the list. Move the highlight with `↑` and `↓`.
3. Press `s` to start it, `x` to stop it. The log fills the panel on the right.
4. Press `c` to open a console and type commands straight to the server.
5. Have many servers? Press `/` to filter the list by name.

Have several servers, each in its own folder? Add each one with `a`. Or point
Beacon at a folder that contains all of them and it picks up every server inside.

The right-hand panel's header shows which script or jar Beacon starts each
server with (`via run.sh`). A pack that ships more than one launcher, such as a
`run.sh` and a `start.sh`, defaults to `run.sh`; press `l` to switch it or to
set the arguments passed to it.

### Keys

| Key     | What it does                                                  |
| ------- | ------------------------------------------------------------- |
| `↑` `↓` | move the highlight                                            |
| `/`     | filter the list by name                                       |
| `s`     | start the highlighted server                                  |
| `x`     | stop it, waiting for a clean shutdown                         |
| `c`     | open a console to type a command straight to the server       |
| `K`     | force-kill (only offered after a stop has hung)               |
| `m`     | mark a server stopped once you have checked it really is down |
| `a`     | add a server (pick its folder)                                |
| `i`     | re-scan your folders for new servers                          |
| `p`     | fix a start script Beacon cannot control (it shows you first) |
| `l`     | launch settings: which script or jar starts it, and its args  |
| `u`     | show the command to update Beacon                             |
| `r`     | refresh now                                                   |
| `?`     | show every key                                                |
| `q`     | quit (your servers keep running)                              |

Only the keys that apply right now show in the bar at the top. `?` expands the
full list.

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
then press `m`.

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

The full design write-up is [`beacon-tui-plan.md`](beacon-tui-plan.md).

### Package layout

| Package                 | Responsibility                                    |
| ----------------------- | ------------------------------------------------- |
| `internal/config`       | dirs, TOML parse, atomic write                    |
| `internal/server`       | IDs, spec, the status state machine               |
| `internal/importdetect` | scan folders, detect scripts and jars, exec patch |
| `internal/supervisor`   | the `Supervisor` port                             |
| `internal/tmux`         | the tmux adapter and the log tailer               |
| `internal/reconcile`    | derive status from tmux, port collision check     |
| `internal/oplock`       | the host operation lock                           |
| `internal/lifecycle`    | start, stop, force-kill under the lock            |
| `internal/selfupdate`   | the startup release check                         |
| `internal/ui`           | the Bubble Tea front end                          |

### Cutting a release

Releases are built by a manually triggered GitHub Actions workflow, not on every
push. In the **Actions** tab, run **Release** and pass a version such as `v0.1.0`.
It cross-compiles `darwin` and `linux` for `amd64` and `arm64`, publishes the
binaries with checksums, and tags the commit.

## License

[MIT](LICENSE)
