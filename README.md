# beacon

A terminal UI for running Java Minecraft servers on macOS and Linux. Beacon imports
servers you already have, keeps each one alive in its own tmux session, tails its log
file, and serializes start/stop so two beacon windows can never launch the same
server twice.

Beacon does not touch networking. However you already reach the box (SSH, Tailscale,
sitting in front of it) is how you reach beacon.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/syoopie/beacon-tui/main/install.sh | bash
```

This downloads a prebuilt binary for your OS and architecture from the latest
[release](https://github.com/syoopie/beacon-tui/releases) and puts it in
`/usr/local/bin` (or `~/.local/bin` if that is not writable).

Pin a version or change the target directory:

```sh
BEACON_VERSION=v0.1.0 BEACON_INSTALL_DIR=~/bin \
  curl -fsSL https://raw.githubusercontent.com/syoopie/beacon-tui/main/install.sh | bash
```

### Prerequisites

- `tmux` and a POSIX shell on `PATH`. macOS: `brew install tmux`. Debian/Ubuntu:
  `sudo apt-get install -y tmux`.
- A Java runtime is the server's concern, not beacon's.

## First run

Beacon needs at least one directory to scan for servers. It will not crawl your home
directory. Create `config.toml` under the config directory
(`~/Library/Application Support/beacon` on macOS, `~/.config/beacon` on Linux):

```toml
scan_roots = ["/absolute/path/to/your/servers"]
stop_timeout = "60s"   # optional, how long a graceful stop waits before offering force-kill
```

Then run `beacon` and press `i` to import. Beacon scans each root and its immediate
subdirectories for a `run.sh` / `start.sh` or a `server.jar` / `paper*.jar` /
`fabric-server*.jar`, and writes one `servers/<id>.toml` per server it finds.

Server state lives under the state directory
(`$XDG_STATE_HOME/beacon` or `~/.local/state/beacon`): the tmux sessions are named
`beacon-<id>`, and each server's log is `logs/<id>.log`.

### Keys

`j` / `k` move, `s` start, `x` stop, `K` force-kill (offered only after a stop times
out), `m` mark a server stopped once you have confirmed an Unknown server is really
down, `i` import, `p` patch a start script that does not `exec` its command, `r`
refresh, `q` quit.

## Updating

On startup beacon asks GitHub whether a newer release is out. If there is one, the
header shows it and `u` prints the install command. Re-running the install one-liner
replaces the binary in place. The check is best-effort and silent on any network
failure.

## Building from source

```sh
go build -o beacon ./cmd/beacon
```

Requires Go 1.24 or newer.

## Cutting a release

Releases are built by a manually triggered GitHub Actions workflow, not on every
push. In the repo's Actions tab, run **Release** and pass a version such as
`v0.1.0`. The workflow cross-compiles `darwin` and `linux` for `amd64` and `arm64`,
publishes them with checksums, and tags the commit.

## License

MIT. See [LICENSE](LICENSE).
