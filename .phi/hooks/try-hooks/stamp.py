#!/usr/bin/env python3
"""PostTool: attach a short model-facing stamp."""

import json
import sys

ev = json.loads(sys.stdin.read() or "{}")
tool = ev.get("tool") or "?"
json.dump({"context": f"stamp: {tool} ran via try-hooks plugin"}, sys.stdout)
sys.stdout.write("\n")
