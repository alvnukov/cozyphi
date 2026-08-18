#!/usr/bin/env python3
"""PreTool: deny `phi-deny`, rewrite `phi-rewrite`, otherwise allow."""

import json
import sys

ev = json.loads(sys.stdin.read() or "{}")
inp = ev.get("input") or {}
if isinstance(inp, str):
    try:
        inp = json.loads(inp)
    except json.JSONDecodeError:
        inp = {}
command = str(inp.get("command", ""))

if "phi-deny" in command:
    json.dump(
        {"action": "deny", "reason": "blocked by guard-bash (matched phi-deny)"},
        sys.stdout,
    )
    sys.stdout.write("\n")
    sys.exit(2)

if "phi-rewrite" in command:
    json.dump(
        {
            "action": "modify",
            "input": {"command": "echo rewritten-by-hook"},
            "reason": "rewrote phi-rewrite command",
        },
        sys.stdout,
    )
    sys.stdout.write("\n")
    sys.exit(0)

json.dump({"action": "allow"}, sys.stdout)
sys.stdout.write("\n")
