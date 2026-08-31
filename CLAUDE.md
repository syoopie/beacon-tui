# Beacon

A Bubble Tea TUI that runs Java Minecraft servers on macOS and Linux under tmux.
Module `github.com/syoopie/beacon-tui`, Go 1.24.

## Checks

Run `make check` before every commit (gofmt, `go vet`, `go test ./...`).
`make lint` runs golangci-lint; CI always runs it, so keep it clean. `make
test-unix` also runs the tmux integration tests (they need `tmux` on PATH and
skip without it). `make run` is `go run ./cmd/beacon`.

## Identity

The only name on any commit, in history, or in a repo file is
`syoopie <48171632+syoopie@users.noreply.github.com>`. The maintainer's real
name and personal email must never appear anywhere in the repo.

Every commit message ends with:

```
Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: <the session URL>
```

## Working agreement

- **Don't push, don't cut releases, don't touch GitHub Actions permissions**
  unless asked. The maintainer reviews local commits first.
- **Staged, reviewable commits.** Large work is split into phases; finish a
  phase, report what it did, and wait for review before the next one.
- **Reuse, don't hand-roll.** If a proven library solves it, use it
  (`gorcon/rcon`, `x/mod/semver`, `dustin/go-humanize`, the bubbles
  components). A hand-rolled version needs a real reason.
- **Few subagents.** Do the work directly; delegation here is expensive.
- Keep replies and commit messages plain. No em-dashes.

## Verifying UI changes

Model tests (`internal/ui/*_test.go` driving `tea.Model`) do **not** reproduce
terminal rendering bugs. lipgloss disables colour when it sees no TTY, so styled
output never runs, and `tm.View()` is not what a terminal actually paints.

For anything about layout, redraw, scrolling, or colour, use the
[`verify-beacon`](.claude/skills/verify-beacon/SKILL.md) skill. It runs the real
binary on a PTY, feeds its bytes through a VT emulator, and hands back the grid
of cells. `check_console.py` there is the regression check for the console.

Two rules the log screen depends on:

- **Sanitize anything from outside the program before measuring it.** A tab is
  one column to `ansi.StringWidth` and up to eight on screen, so an unsanitized
  log line is drawn wider than it was laid out for. `sanitize` in
  `internal/ui/follow.go` is the single place that happens.
- **Wrap once.** Content already wrapped by `ansi.Wrap` must not be passed
  through a lipgloss `Width` style, which wraps again and leaves ragged
  fragments.

Read captured grids with a script, never by eye: a proportional font makes
aligned columns look ragged and ragged ones look aligned. Real Minecraft logs
for fixtures: `~/MinecraftServer/BMC4_ServerPack_v61/logs/latest.log`.

## Architecture

- **Disk is the source of truth.** `config.toml` and `servers/<id>.toml` hold
  everything. Every process reloads them each tick; memory is a stale cache
  until the next reconcile.
- **tmux owns process lifetime only.** Each server runs as `beacon-<id>`, its
  shell `exec`s the launch command so the pane PID is the JVM.
- **A host lockfile serializes every mutating op** (start, stop, force-kill,
  import, script patch, config write) across all beacon processes. Reads never
  lock.
- **`internal/supervisor`** is the port; `internal/tmux` is the only adapter.
  Nothing outside `internal/tmux` knows tmux exists.

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
