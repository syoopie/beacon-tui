#!/usr/bin/env python3
"""Drive the Beacon binary in a real PTY and capture the cells a terminal shows.

Beacon is a Bubble Tea TUI. Go tests that call tea.Model.View() do not reproduce
rendering bugs: lipgloss drops colour when it sees no TTY, and View() is a string
the renderer then diffs against the previous frame rather than the bytes a
terminal receives. This runs the real binary on a real PTY, feeds every byte it
writes into pyte (a VT100/ECMA-48 emulator that measures character width the way
terminals do), and prints the resulting grid.

  ./drive.py --cols 123 --rows 40 --server ~/MinecraftServer/PACK \
      key:right snap:console key:up*10 snap:scrolled

Each snap writes evidence/<label>.txt. Steps run in order:

  key:<name>[*N]  send a key N times (default 1). Names: up down left right
                  enter esc tab space bs, or any literal character.
  wait:<seconds>  let the app settle and keep reading output.
  snap:<label>    record the current screen.
"""

import argparse
import errno
import fcntl
import os
import pty
import select
import shutil
import signal
import struct
import sys
import termios
import time

import pyte

KEYS = {
    "up": "\x1b[A",
    "down": "\x1b[B",
    "right": "\x1b[C",
    "left": "\x1b[D",
    "enter": "\r",
    "esc": "\x1b",
    "tab": "\t",
    "space": " ",
    "bs": "\x7f",
    "ctrl+f": "\x06",
    "ctrl+r": "\x12",
}

SETTLE = 0.35


class Session:
    def __init__(self, argv, cols, rows, env):
        self.cols, self.rows = cols, rows
        self.screen = pyte.Screen(cols, rows)
        self.stream = pyte.ByteStream(self.screen)
        self.raw = bytearray()
        self.pid, self.fd = pty.fork()
        if self.pid == 0:
            os.environ.update(env)
            os.execvp(argv[0], argv)
        fcntl.ioctl(self.fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))

    def pump(self, seconds):
        end = time.monotonic() + seconds
        while True:
            left = end - time.monotonic()
            if left <= 0:
                return
            if not select.select([self.fd], [], [], left)[0]:
                continue
            try:
                chunk = os.read(self.fd, 65536)
            except OSError as e:
                if e.errno in (errno.EIO, errno.EBADF):
                    return
                raise
            if not chunk:
                return
            self.raw += chunk
            self.stream.feed(chunk)
            self.answer(chunk)

    # Bubble Tea asks the terminal about itself on startup and blocks until it
    # answers. pyte only models the screen, so we play the terminal's half:
    # cursor position, background colour, device attributes, kitty keyboard.
    QUERIES = [
        (b"\x1b[6n", b"\x1b[1;1R"),
        (b"\x1b]11;?", b"\x1b]11;rgb:0000/0000/0000\x1b\\"),
        (b"\x1b[c", b"\x1b[?1;2c"),
        (b"\x1b[>c", b"\x1b[>0;95;0c"),
        (b"\x1b[?u", b"\x1b[?0u"),
    ]

    def answer(self, chunk):
        for query, reply in self.QUERIES:
            for _ in range(chunk.count(query)):
                os.write(self.fd, reply)

    def send(self, text):
        os.write(self.fd, text.encode())

    def display(self):
        return [line.rstrip() for line in self.screen.display]

    def close(self):
        try:
            os.write(self.fd, b"\x03")  # ctrl+c: Beacon's only quit key
            self.pump(0.3)
        except OSError:
            pass
        for sig in (signal.SIGTERM, signal.SIGKILL):
            try:
                os.kill(self.pid, sig)
            except ProcessLookupError:
                break
            time.sleep(0.2)
            if os.waitpid(self.pid, os.WNOHANG)[0]:
                break
        try:
            os.close(self.fd)
        except OSError:
            pass


def run(args):
    binary = args.binary
    argv = [binary, "--config-dir", args.config_dir, "--state-dir", args.state_dir]
    if args.server:
        argv.append(args.server)

    env = {
        "TERM": "xterm-256color",
        "COLORTERM": "truecolor",
        "LINES": str(args.rows),
        "COLUMNS": str(args.cols),
    }
    s = Session(argv, args.cols, args.rows, env)
    snaps = {}
    try:
        s.pump(args.startup)
        for step in args.steps:
            kind, _, arg = step.partition(":")
            if kind == "key":
                name, _, count = arg.partition("*")
                seq = KEYS.get(name, name)
                for _ in range(int(count or 1)):
                    s.send(seq)
                    s.pump(args.settle)
            elif kind == "wait":
                s.pump(float(arg))
            elif kind == "snap":
                s.pump(args.settle)
                snaps[arg] = s.display()
            else:
                sys.exit(f"unknown step: {step}")
    finally:
        s.close()

    os.makedirs(args.evidence, exist_ok=True)
    with open(os.path.join(args.evidence, "raw.bin"), "wb") as f:
        f.write(bytes(s.raw))
    for label, lines in snaps.items():
        path = os.path.join(args.evidence, label + ".txt")
        with open(path, "w") as f:
            f.write("\n".join(lines) + "\n")
        print(f"=== {label} ({args.cols}x{args.rows}) -> {path}")
        for i, line in enumerate(lines):
            print(f"{i:3} |{line}")
        print()
    return snaps


def main():
    p = argparse.ArgumentParser()
    p.add_argument("steps", nargs="*")
    p.add_argument("--binary", default="/tmp/beacon-verify")
    p.add_argument("--cols", type=int, default=123)
    p.add_argument("--rows", type=int, default=40)
    p.add_argument("--server", default="")
    p.add_argument("--config-dir", default="")
    p.add_argument("--state-dir", default="")
    p.add_argument("--evidence", default="evidence")
    p.add_argument("--startup", type=float, default=1.2)
    p.add_argument("--settle", type=float, default=SETTLE)
    args = p.parse_args()

    if not args.config_dir or not args.state_dir:
        base = os.path.join(args.evidence, "home")
        args.config_dir = args.config_dir or os.path.join(base, "config")
        args.state_dir = args.state_dir or os.path.join(base, "state")
        shutil.rmtree(base, ignore_errors=True)
        os.makedirs(args.config_dir, exist_ok=True)
        os.makedirs(args.state_dir, exist_ok=True)
    run(args)


if __name__ == "__main__":
    main()
