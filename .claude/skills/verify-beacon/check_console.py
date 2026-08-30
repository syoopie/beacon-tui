#!/usr/bin/env python3
"""Assert the console's side rail holds one column while the log scrolls.

This is the regression check for the bug that took three attempts to find: a log
line whose real width on screen differs from the width Beacon measured pushes
the rail sideways on exactly the rows that carry one. Tabs in Minecraft stack
traces did it. Escape sequences or a wide rune would do it again.

  ./check_console.py --config-dir W/config --state-dir W/state

Exits non-zero and prints the offending frame on failure.
"""

import argparse
import sys

import drive

RAIL_BORDER = "│"


def rail_columns(rows):
    """Columns holding the rail's left border, ignoring the list screen."""
    cols = {}
    for i, row in enumerate(rows):
        if RAIL_BORDER in row:
            cols.setdefault(row.index(RAIL_BORDER), []).append(i)
    return cols


def probe(session, label, failures):
    rows = session.display()
    cols = rail_columns(rows)
    if not cols:
        failures.append((label, "no rail border on screen", rows))
    elif len(cols) > 1:
        failures.append((label, f"rail border spans columns {sorted(cols)}", rows))
    over = [r for r in rows if len(r) > session.cols]
    if over:
        failures.append((label, f"{len(over)} rows wider than the terminal", rows))


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--binary", default="/tmp/beacon-verify")
    p.add_argument("--config-dir", required=True)
    p.add_argument("--state-dir", required=True)
    p.add_argument("--widths", default="72,90,110,123,160")
    p.add_argument("--rows", type=int, default=40)
    p.add_argument("--scrolls", type=int, default=40)
    p.add_argument("--settle", type=float, default=drive.SETTLE)
    args = p.parse_args()

    failures = []
    for cols in [int(c) for c in args.widths.split(",")]:
        argv = [args.binary, "--config-dir", args.config_dir, "--state-dir", args.state_dir]
        env = {"TERM": "xterm-256color", "COLORTERM": "truecolor",
               "LINES": str(args.rows), "COLUMNS": str(cols)}
        s = drive.Session(argv, cols, args.rows, env)
        try:
            s.pump(1.2)
            for k in ("right", "enter"):
                s.send(drive.KEYS[k])
                s.pump(0.35)
            for full in (False, True):
                if full:
                    s.send("f")
                    s.pump(0.35)
                for step in range(args.scrolls):
                    s.pump(args.settle)
                    probe(s, f"{cols}x{args.rows} full={full} step={step}", failures)
                    s.send(drive.KEYS["up"])
        finally:
            s.close()
        print(f"{cols}x{args.rows}: {args.scrolls * 2} frames checked")

    if failures:
        label, why, rows = failures[0]
        print(f"\nFAIL {label}: {why}\n", file=sys.stderr)
        for i, row in enumerate(rows):
            print(f"{i:3} |{row}", file=sys.stderr)
        print(f"\n{len(failures)} failing frames", file=sys.stderr)
        return 1
    print("\nPASS: the rail held one column in every frame")
    return 0


if __name__ == "__main__":
    sys.exit(main())
