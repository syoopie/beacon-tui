# beacon

A terminal UI for running Java Minecraft servers on macOS and Linux. Import the
servers you already have, keep each one alive in its own tmux session, watch its
log, and start or stop it safely even with several beacon windows open at once.

[![CI](https://github.com/syoopie/beacon-tui/actions/workflows/ci.yml/badge.svg)](https://github.com/syoopie/beacon-tui/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/syoopie/beacon-tui?sort=semver)](https://github.com/syoopie/beacon-tui/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/syoopie/beacon-tui)](go.mod)
[![License](https://img.shields.io/github/license/syoopie/beacon-tui)](LICENSE)

Beacon does not touch networking. However you already reach the machine (SSH,
Tailscale, or sitting in front of it) is how you reach beacon.

```text
 beacon   ⬆ v0.2.0 available (press u for the command)
┌────────────────────────────┬──────────────────────────────────────────────────┐
│ ▶ survival        running  │ [12:31:02] [Server thread/INFO]: Done (6.4s)!     │
│ ○ creative        stopped  │ [12:31:02] [Server thread/INFO]: For help, type   │
│ ? skyblock        unknown  │   "help"                                          │
│                            │ [12:33:20] [Server thread/INFO]: Steve joined the │
│                            │   game                                           │
├────────────────────────────┴──────────────────────────────────────────────────┤
│ survival  [running]  port 25565                                               │
│ j/k move · s start · x stop · K force-kill · a add-folder · i import · q quit  │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Features

- One binary, no runtime dependencies except `tmux` and a POSIX shell.
- Imports servers started by `run.sh` / `start.sh` or a `server.jar` /
  `paper*.jar` / `fabric-server*.jar`.
- Servers keep running after you close beacon. Each lives in a `beacon-<id>` tmux
  session.
- Reconciles recorded state against tmux on a tick. A server whose session
  vanished shows as `unknown`, never a silent `stopped`, so you cannot double
  launch it into a port collision.
- One mutating operation at a time across every beacon process, via a host lock.
  A second window trying to start the same server is told an operation is in
  progress instead of launching a second JVM.
- Graceful stop with a timeout, then force-kill is offered. Beacon never
  force-kills on its own.
- Checks for a newer release on startup and shows the update command.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/syoopie/beacon-tui/main/install.sh | bash
```

Downloads a prebuilt binary for your OS and architecture from the latest
[release](https://github.com/syoopie/beacon-tui/releases), verifies its checksum,
and installs it to `/usr/local/bin` (or `~/.local/bin` if that is not writable).

Pin a version or change the target directory:

```sh
BEACON_VERSION=v0.1.0 BEACON_INSTALL_DIR=~/bin \
  curl -fsSL https://raw.githubusercontent.com/syoopie/beacon-tui/main/install.sh | bash
```

With a Go toolchain you can instead `go install
github.com/syoopie/beacon-tui/cmd/beacon@latest`.

**Prerequisite:** `tmux` on `PATH`. macOS: `brew install tmux`. Debian/Ubuntu:
`sudo apt-get install -y tmux`. A Java runtime is your server's concern, not
beacon's.

## Quick start

```sh
beacon
```

1. Press `a` and enter the folder your server directories live in.
2. Beacon scans it and lists what it finds. Move with `j` / `k`.
3. Press `s` to start the selected server, `x` to stop it. Its log fills the
   right pane.

`beacon /path/to/servers` does step 1 for you.

### Keys

| Key     | Action                                                        |
| ------- | ------------------------------------------------------------- |
| `j` `k` | move the selection                                            |
| `s`     | start the selected server                                     |
| `x`     | stop it, waiting up to the stop timeout                       |
| `K`     | force-kill (offered only after a stop times out)              |
| `m`     | mark an `unknown` server stopped, once you confirm it is down |
| `a`     | add a folder to scan                                          |
| `i`     | re-scan the configured folders                                |
| `p`     | patch a start script that does not `exec` its command         |
| `u`     | show the update command when a new release is available       |
| `r`     | refresh now                                                   |
| `q`     | quit (servers keep running)                                   |

## How it works

- **tmux owns process lifetime and stdin.** Beacon launches each server through a
  shell that redirects output to a log file and `exec`s the command, so the pane
  process is the JVM itself.
- **Logs are files**, tailed from disk. Beacon reopens them if you truncate them.
- **`config.toml` and `servers/<id>.toml` on disk are the source of truth.** Every
  beacon process reloads them each tick. Memory is a cache.
- **A host lock serializes every mutating operation** (start, stop, force-kill,
  import, script patch, config write) across all beacon processes. It is a
  PID-bearing lockfile with stale-holder recovery. Reads never take it.
- **Reconcile** compares the recorded state to tmux on startup and on a ticker.

The full design is in [`beacon-tui-plan.md`](beacon-tui-plan.md).

## Configuration

You rarely need to edit this by hand. It lives at
`config.toml` under the config directory (`~/Library/Application Support/beacon`
on macOS, `~/.config/beacon` on Linux):

```toml
scan_roots   = ["/absolute/path/to/your/servers"]
stop_timeout = "60s"   # how long a graceful stop waits before force-kill is offered
```

Server state and logs live under the state directory (`$XDG_STATE_HOME/beacon`
or `~/.local/state/beacon`): tmux sessions are `beacon-<id>`, logs are
`logs/<id>.log`.

## Updating

On startup beacon asks GitHub whether a newer release exists. If one is out, the
header shows it and `u` prints the install command. Re-run the one-liner to
replace the binary in place. The check is best-effort and silent on any network
failure.

## Development

Requires Go 1.24+ and `tmux`.

```sh
git clone https://github.com/syoopie/beacon-tui
cd beacon-tui
make check        # gofmt, go vet, go test
make test-unix    # adds the tmux integration tests
make run          # go run ./cmd/beacon
```

The tmux integration tests carry a `//go:build unix` tag and skip when `tmux` is
not on `PATH`. `make lint` runs golangci-lint if you have it installed; CI always
does.

Package layout:

| Package                 | Responsibility                                    |
| ----------------------- | ------------------------------------------------- |
| `internal/config`       | dirs, TOML parse and atomic write                 |
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
push. In the repo's **Actions** tab, run **Release** and pass a version such as
`v0.1.0`. It cross-compiles `darwin` and `linux` for `amd64` and `arm64`,
publishes the binaries with checksums, and tags the commit.

## License

[MIT](LICENSE)
