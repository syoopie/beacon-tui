---
name: verify-beacon
description: Drive the real Beacon binary in a PTY and read the cells a terminal actually paints. Use this for any change to internal/ui - layout, wrapping, scrolling, colour, redraw - and before claiming a rendering bug is fixed. Go tests that assert on tea.Model.View() cannot see these bugs.
---

# Verifying Beacon

Beacon is a Bubble Tea TUI. It has one user-facing surface: a full-screen
terminal app started as `beacon [scan-root]`.

**Go model tests are not evidence for anything visual.** They drive `tea.Model`
and read `View()`, which is a string the renderer then diffs against the
previous frame. Three separate "fixes" to a rail-alignment bug passed those
tests while the bug was still on screen. Two reasons: lipgloss drops all colour
when it sees no TTY, so the styled code path never runs, and `View()` is not the
byte stream a terminal receives.

This skill runs the real binary on a real PTY and feeds every byte it writes
into [pyte](https://github.com/selectel/pyte), a VT100/ECMA-48 emulator that
measures character width the way terminals do. What you get back is the grid of
cells, which is what the user is looking at.

## Launch

One-time setup (the venv lives in the scratchpad, not the repo):

```sh
python3 -m venv /tmp/beacon-verify-venv
/tmp/beacon-verify-venv/bin/pip install pyte
```

Build the binary under test, and build a fixture so the console has a real log
to render. Minecraft logs are the interesting input: they carry tabs, escape
sequences and lines far wider than any column.

```sh
cd <repo>
go build -o /tmp/beacon-verify ./cmd/beacon

W=/tmp/beacon-fixture
rm -rf $W && mkdir -p $W/config/servers $W/state/logs
cp ~/Library/Application\ Support/beacon/config.toml            $W/config/
cp ~/Library/Application\ Support/beacon/servers/*.toml         $W/config/servers/
cp ~/MinecraftServer/BMC4_ServerPack_v61/logs/latest.log        $W/state/logs/bmc4_serverpack_v61.log
sed -i '' "s#log_file = \".*\"#log_file = \"$W/state/logs/bmc4_serverpack_v61.log\"#" $W/config/servers/*.toml
```

There is no server to keep alive. Every drive forks its own PTY, and
`Session.close()` kills it. Copy `$W` to a scratch directory per run if the
drive will write config, so the fixture stays pristine.

## Doctor

```sh
/tmp/beacon-verify --version                     # right build?
/tmp/beacon-verify-venv/bin/python -c "import pyte"   # emulator present?
grep log_file /tmp/beacon-fixture/config/servers/*.toml   # points into the fixture?
```

If a drive comes back with a blank screen, the responder is the thing to
suspect. Bubble Tea asks the terminal about itself on startup (cursor position,
background colour, device attributes) and blocks until answered. `drive.py`
answers those; a new Bubble Tea version may add a query it does not know.
`evidence/raw.bin` ending in an unanswered `\x1b[...n` or `\x1b]...?` is the
tell, and the fix is another entry in `Session.QUERIES`.

## Drive

`drive.py` takes a list of steps and prints the screen at each `snap:`.

```sh
cd /tmp && PYTHONPATH=<repo>/.claude/skills/verify-beacon \
  /tmp/beacon-verify-venv/bin/python <repo>/.claude/skills/verify-beacon/drive.py \
  --cols 123 --rows 40 \
  --config-dir /tmp/beacon-fixture/config --state-dir /tmp/beacon-fixture/state \
  key:right key:enter snap:console key:f 'key:up*8' snap:scrolled
```

Steps:

| Step             | Meaning                                                       |
| ---------------- | ------------------------------------------------------------- |
| `key:<name>[*N]` | send a key N times: `up down left right enter esc tab space bs`, or any literal character |
| `wait:<seconds>` | let the app settle and keep reading                            |
| `snap:<label>`   | record the screen to `evidence/<label>.txt` and print it       |

Quote `key:up*8` in zsh, or the glob expands.

Widths matter. Run at least 72, 110 and 160 columns: the console only shows its
side rail above 64 inner columns, and the help bar reflows.

## Evidence

`drive.py` writes to `./evidence/`: one `<label>.txt` per snapshot holding the
exact cell grid, plus `raw.bin`, every byte the binary wrote. Cleanup never
touches `evidence/`.

Read the grid with a script, not with your eyes. A proportional font in a chat
transcript makes aligned columns look ragged and ragged columns look aligned;
that mistake cost a full debugging round here.

```sh
python3 -c "
for i, l in enumerate(open('evidence/scrolled.txt')):
    l = l.rstrip('\n')
    if '│' in l: print(i, l.index('│'))"
```

Proof standards: drive the real key sequence a user would press, snapshot the
state before and after the action rather than only the end, and when a change
touches wrapping or measurement, use a fixture line that actually contains the
hard case (a tab, an escape sequence, an unbreakable 120-character token).

## check_console.py

The regression check for the console screen. It opens the console at several
widths, scrolls the log in both filter modes, and asserts the rail's left border
holds a single column in every frame.

```sh
cd /tmp && PYTHONPATH=<repo>/.claude/skills/verify-beacon \
  /tmp/beacon-verify-venv/bin/python <repo>/.claude/skills/verify-beacon/check_console.py \
  --config-dir /tmp/beacon-fixture/config --state-dir /tmp/beacon-fixture/state \
  --widths 72,90,110,123,160 --scrolls 20
```

It prints the offending frame and exits non-zero on failure.

## Cleanup

`drive.py` and `check_console.py` kill the PTY child they forked, by pid, in a
`finally`. They never kill by name, so a Beacon the user is running by hand is
safe. If a drive is interrupted hard, `pgrep -f beacon-verify` finds the strays;
the user's own `beacon` has a different path.

Beacon starts tmux sessions named `beacon-<id>` when a server is started. Do not
drive Start against the fixture unless you mean to, and tear down with
`tmux kill-session -t beacon-<id>`.

Remove `/tmp/beacon-fixture` and its per-run copies when done. Leave
`evidence/`.

## Feature map

`features/` is the list of what a user can do and how to drive each one. A proof
that exercises one convenient entry point is incomplete when the map names
others. Keep it current as the UI changes.
