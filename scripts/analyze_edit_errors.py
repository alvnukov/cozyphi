#!/usr/bin/env python3
"""Analyze model edit/write mistakes in cozyphi session transcripts.

Walks cozyphi session transcripts (main sessions under <root>/session/ and
sub-agent jobs under <root>/jobs/<id>/session/), pairs tool calls with their
role="tool" results, classifies failures of edit/write/read(mode=edit) and
prints aggregate frequencies, retry chains and sanitized examples.

Only the Python standard library is used. The script never prints tool
arguments or file contents — only paths, line/hash references and short
snippets of tool-generated error texts (paths, numbers, hashes), so no
secrets from transcripts can reach the report.

Usage:
    python3 scripts/analyze_edit_errors.py [--root ~/.cozyphi] [--examples N] [--json]
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from collections import Counter
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

TRACKED_TOOLS = ("edit", "write", "read")

SNIPPET_LEN = 200

# Success shapes, per tool, from the tool implementations:
#   read    -> "@read <path> (N lines, ...)"
#   edit    -> "@file <path>#TAG ..." plus "Re-read this file before another edit"
#   write   -> "wrote N bytes to <path>"
SUCCESS_SHAPES = {
    "read": (re.compile(r"^@file "),),  # mode=edit returns the @file path#TAG header
    "edit": (re.compile(r"^@file "),),
    "write": (re.compile(r"^wrote \d+ bytes to "),),
    "read_view": (re.compile(r"^@read "), re.compile(r"^@file "), re.compile(r"^\d+\|")),  # header formats changed over time
}

CANCELED_TEXT = "User cancelled the tool call."

# Ordered error classifiers: (class, predicate over the lowercased content).
# Order matters — the first match wins; keep specific patterns above generic.
ERROR_CLASSES: list[tuple[str, Any]] = [
    # Harness withheld the call itself (skill preload choreography): not the model's fault.
    ("withheld_retry", lambda c: "this tool was not executed" in c),
    ("context_limit", lambda c: c.startswith("context limit reached")),  # prefix: file content may quote the phrase
    ("plan_gate", lambda c: (
        "is not allowed on a" in c
        or "not a valid step in the approved plan" in c
        or "not an active step" in c
        or "the plan is not approved" in c
        or "in_progress items" in c
    )),
    ("no_capability", lambda c: "not authorized by a current-session editable read" in c),
    ("missing_hash", lambda c: "edit requires hash" in c),
    ("tag_mismatch", lambda c: "file tag mismatch" in c),
    ("changed_during_edit", lambda c: "file changed during edit" in c),
    ("stale_anchors", lambda c: "have changed since last read" in c),
    ("out_of_bounds", lambda c: "out of bounds" in c),
    ("bad_range", lambda c: "range start line" in c),
    ("overlap", lambda c: "edits overlap" in c),
    ("invalid_ref", lambda c: "invalid line reference" in c or "single line#hash" in c),
    ("missing_anchors", lambda c: "requires non-empty from and to" in c),
    ("missing_path", lambda c: "requires a non-empty path" in c or "path is required" in c),
    ("parse_args", lambda c: "failed to parse" in c),
    ("read_too_big", lambda c: "refuse to hash" in c),
    ("invalid_mode", lambda c: "invalid read mode" in c),
    ("outside_workspace", lambda c: "outside workspace" in c or "outside the workspace" in c),
    ("permission", lambda c: "permission check failed" in c or "denied by user" in c),
    ("tool_not_found", lambda c: re.search(r"tool '\S+' not found", c) is not None),
    ("write_failed", lambda c: "failed to write file" in c),
    ("fs_error", lambda c: "no such file or directory" in c or "is a directory" in c or "permission denied" in c),
]


@dataclass
class Call:
    """A tracked tool call paired with its result, if one was recorded."""

    session: Path
    origin: str  # "session" | "job"
    tool: str  # edit | write | read (mode=edit) | read_view
    args: dict[str, Any] = field(default_factory=dict)
    ts: str = ""
    path: str = ""
    status: str = "no_result"  # success | error | canceled | unknown | no_result
    klass: str = ""
    snippet: str = ""
    blind: bool = False  # edit retried on a path whose last edit failed without a re-read


def content_to_text(content: Any) -> str:
    """Flatten provider content shapes (string or block list) to text."""
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        parts = []
        for block in content:
            if isinstance(block, dict):
                text = block.get("text") or block.get("content") or ""
            else:
                text = str(block)
            if isinstance(text, str):
                parts.append(text)
        return "\n".join(parts)
    return ""


def classify(tool: str, content: str) -> tuple[str, str]:
    """Return (status, class): status is success|error|canceled|unknown."""
    if content == CANCELED_TEXT:
        return "canceled", "canceled"
    if any(rx.match(content) for rx in SUCCESS_SHAPES.get(tool, ())):
        return "success", ""
    lowered = content.lower()
    for klass, predicate in ERROR_CLASSES:
        if predicate(lowered):
            return "error", klass
    return "unknown", ""


def session_files(root: Path) -> list[tuple[Path, str]]:
    """All transcript files under root as (file, origin)."""
    files: list[tuple[Path, str]] = []
    session_dir = root / "session"
    if session_dir.is_dir():
        files.extend((p, "session") for p in sorted(session_dir.rglob("*.jsonl")))
    jobs_dir = root / "jobs"
    if jobs_dir.is_dir():
        for job in sorted(jobs_dir.iterdir()):
            if job.is_dir():
                files.extend((p, "job") for p in sorted((job / "session").glob("*.jsonl")))
    return files


def parse_session(path: Path, origin: str) -> tuple[list[Call], int, int]:
    """Parse one transcript; returns (tracked calls in log order, entries, bad lines)."""
    calls: dict[str, Call] = {}
    ordered: list[Call] = []
    entries = 0
    bad = 0
    with path.open(encoding="utf-8", errors="replace") as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            try:
                entry = json.loads(line)
            except json.JSONDecodeError:
                bad += 1
                continue
            if entry.get("type") != "EntryMessage":
                continue
            entries += 1
            msg = entry.get("message") or {}
            role = msg.get("role")
            if role == "assistant":
                for tc in msg.get("tool_calls") or []:
                    fn = (tc or {}).get("function") or {}
                    name = str(fn.get("name") or "")
                    if name not in TRACKED_TOOLS:
                        continue
                    try:
                        args = json.loads(fn.get("arguments") or "{}")
                    except json.JSONDecodeError:
                        args = {}
                    if not isinstance(args, dict):
                        args = {}
                    tool = name
                    if tool == "read" and str(args.get("mode", "")).lower() != "edit":
                        tool = "read_view"
                    call = Call(
                        session=path,
                        origin=origin,
                        tool=tool,
                        args=args,
                        ts=str(entry.get("timestamp") or ""),
                        path=str(args.get("path") or "").strip(),
                    )
                    calls[str(tc.get("id") or "")] = call
                    ordered.append(call)
            elif role == "tool":
                cid = str(msg.get("tool_call_id") or entry.get("tool_call_id") or "")
                call = calls.get(cid)
                if call is None or call.status != "no_result":
                    continue  # unmatched id or a replayed result from a branch
                text = content_to_text(msg.get("content"))
                call.status, call.klass = classify(call.tool, text)
                if call.status in ("error", "unknown"):
                    call.snippet = " ".join(text.split())[:SNIPPET_LEN]
    return ordered, entries, bad


def group_by_session(calls: list[Call]) -> dict[Path, list[Call]]:
    out: dict[Path, list[Call]] = {}
    for c in calls:
        out.setdefault(c.session, []).append(c)
    return out


def analyze_chains(calls: list[Call]) -> list[dict[str, Any]]:
    """Per-session, per-path edit retry chains.

    A chain is every edit attempt on one path inside one session (approximation:
    separate touches of the same file merge). A retry is blind when the previous
    edit on that path failed and no read(mode=edit) of it happened in between.
    """
    chains: list[dict[str, Any]] = []
    for session, evs in group_by_session(calls).items():
        last_failed: dict[str, bool] = {}
        groups: dict[str, list[Call]] = {}
        for c in evs:
            p = c.path
            if not p:
                continue
            if c.tool == "read":
                last_failed[p] = False  # editable read refreshes the anchors
                continue
            if c.tool != "edit":
                continue
            groups.setdefault(p, []).append(c)
            if last_failed.get(p):
                c.blind = True
            last_failed[p] = c.status == "error"
        for p, group in groups.items():
            chains.append(
                {
                    "session": session.name,
                    "origin": group[0].origin,
                    "path": p,
                    "attempts": len(group),
                    "errors": sum(1 for c in group if c.status == "error"),
                    "blind_retries": sum(1 for c in group if c.blind),
                    "succeeded": any(c.status == "success" for c in group),
                }
            )
    return chains


def scope_of(path: str) -> str:
    if ".worktrees/" in path:
        return "worktree"
    if path.startswith("/") or path.startswith("~"):
        return "absolute"
    return "cwd-relative"


def analyze(root: Path, examples: int, debug_unknown: int = 0) -> dict[str, Any]:
    files = session_files(root)
    all_calls: list[Call] = []
    entries = bad = 0
    n_sessions = n_jobs = 0
    for path, origin in files:
        if origin == "job":
            n_jobs += 1
        else:
            n_sessions += 1
        calls, e, b = parse_session(path, origin)
        entries += e
        bad += b
        all_calls.extend(calls)

    by_tool: dict[str, list[Call]] = {}
    for c in all_calls:
        by_tool.setdefault(c.tool, []).append(c)

    class_counts: Counter[str] = Counter()
    per_tool_errors: dict[str, Counter[str]] = {}
    examples_by_class: dict[str, list[Call]] = {}
    # For anchor errors: was the failing call a single edit or a multi-edit batch?
    # Counts only (len of the edits array), never edit contents.
    anchor_batch = {"single": 0, "multi": 0}
    for tool in ("edit", "write", "read"):
        for c in by_tool.get(tool, []):
            if c.status == "canceled":
                class_counts["canceled"] += 1
                per_tool_errors.setdefault(tool, Counter())["canceled"] += 1
            elif c.status == "error":
                klass = c.klass or "unclassified"
                class_counts[klass] += 1
                per_tool_errors.setdefault(tool, Counter())[klass] += 1
                examples_by_class.setdefault(klass, []).append(c)
                if klass in ("stale_anchors", "tag_mismatch", "no_capability") and c.tool == "edit":
                    edits = c.args.get("edits")
                    anchor_batch["multi" if isinstance(edits, list) and len(edits) > 1 else "single"] += 1

    chains = analyze_chains(all_calls)
    total_chains = len(chains)
    first_try = sum(1 for ch in chains if ch["attempts"] == 1 and ch["succeeded"])
    retried = sum(1 for ch in chains if ch["attempts"] > 1)
    blind_total = sum(ch["blind_retries"] for ch in chains)

    # Paths where an edit failed and a write to the same path also happened in
    # the same session (order not checked — approximation of edit→write fallback).
    edit_fail_then_write = 0
    write_after_edit_ok = 0
    for session, group in group_by_session(all_calls).items():
        edit_paths = {c.path for c in group if c.tool == "edit" and c.path}
        failed = {c.path for c in group if c.tool == "edit" and c.status == "error" and c.path}
        wrote = {c.path for c in group if c.tool == "write" and c.path}
        edit_fail_then_write += len(failed & wrote)
        write_after_edit_ok += len((edit_paths - failed) & wrote)

    scope = Counter(
        scope_of(c.path)
        for c in by_tool.get("edit", []) + by_tool.get("write", [])
        if c.path
    )

    # Edit outcomes per session date (YYYY-MM-DD from the file name) — shows whether
    # error rates changed as the harness evolved.
    by_date: dict[str, list[int]] = {}
    for c in by_tool.get("edit", []):
        day = c.session.name[:10]
        slot = by_date.setdefault(day, [0, 0])
        slot[0] += 1
        if c.status == "error":
            slot[1] += 1
    timeline = {
        day: {"calls": v[0], "err": v[1], "rate_pct": round(100.0 * v[1] / v[0], 1)}
        for day, v in sorted(by_date.items())
    }

    return {
        "root": str(root),
        "corpus": {
            "files": len(files),
            "sessions": n_sessions,
            "jobs": n_jobs,
            "entries": entries,
            "bad_lines": bad,
        },
        "tools": {
            tool: {
                "calls": len(cs),
                "success": sum(1 for c in cs if c.status == "success"),
                "error": sum(1 for c in cs if c.status == "error"),
                "canceled": sum(1 for c in cs if c.status == "canceled"),
                "unknown": sum(1 for c in cs if c.status == "unknown"),
                "no_result": sum(1 for c in cs if c.status == "no_result"),
            }
            for tool, cs in sorted(by_tool.items())
        },
        "classes": dict(class_counts.most_common()),
        "per_tool_errors": {t: dict(v.most_common()) for t, v in sorted(per_tool_errors.items())},
        "chains": {
            "total": total_chains,
            "first_try_success": first_try,
            "retried": retried,
            "blind_retries": blind_total,
            "worst": sorted(chains, key=lambda ch: (-ch["errors"], -ch["attempts"]))[:10],
        },
        "edit_fail_then_write": edit_fail_then_write,
        "write_after_edit_ok": write_after_edit_ok,
        "scope": dict(scope),
        "edit_errors_by_date": timeline,
        "anchor_batch": anchor_batch,
        "unknown_samples": {
            tool: [
                {"session": c.session.name, "path": c.path, "snippet": c.snippet[:160]}
                for c in cs
                if c.status == "unknown"
            ][:debug_unknown]
            for tool, cs in sorted(by_tool.items())
            if debug_unknown
        },
        "examples": {
            klass: [
                {
                    "session": c.session.name,
                    "origin": c.origin,
                    "tool": c.tool,
                    "path": c.path,
                    "snippet": c.snippet,
                }
                for c in calls[:examples]
            ]
            for klass, calls in sorted(examples_by_class.items())
        },
    }


def print_report(data: dict[str, Any]) -> None:
    corpus = data["corpus"]
    print(f"corpus: {data['root']}")
    print(
        f"  files={corpus['files']} (sessions={corpus['sessions']}, jobs={corpus['jobs']}), "
        f"entries={corpus['entries']}, bad_lines={corpus['bad_lines']}"
    )
    print("\ntool outcomes:")
    for tool, st in data["tools"].items():
        total, err = st["calls"], st["error"]
        rate = (100.0 * err / total) if total else 0.0
        print(
            f"  {tool:<10} calls={total:<5} ok={st['success']:<5} err={err:<4} "
            f"({rate:.1f}%) canceled={st['canceled']} unknown={st['unknown']} no_result={st['no_result']}"
        )
    print("\nerror classes (edit/write/read-edit, incl. canceled):")
    total_err = sum(data["classes"].values()) or 1
    for klass, count in data["classes"].items():
        print(f"  {klass:<20} {count:<5} ({100.0 * count / total_err:.1f}%)")
    print("\nerror classes per tool:")
    for tool, classes in data["per_tool_errors"].items():
        print(f"  {tool}: {classes}")
    chains = data["chains"]
    print(f"\nanchor errors by edits-in-call (stale/mismatch/no_capability): {data['anchor_batch']}")
    print("\nretry chains (edit, per path per session):")
    print(
        f"  chains={chains['total']} first_try_success={chains['first_try_success']} "
        f"retried={chains['retried']} blind_retries={chains['blind_retries']}"
    )
    for ch in chains["worst"]:
        if ch["errors"] == 0:
            continue
        print(
            f"    {ch['errors']} err / {ch['attempts']} tries  blind={ch['blind_retries']}  "
            f"{ch['session'][:28]}  {ch['path']}"
        )
    print(f"\npaths: edit failed + write in same session: {data['edit_fail_then_write']}")
    print(f"paths: write after ok edit (whole-file rewrite): {data['write_after_edit_ok']}")
    print(f"edit/write path scope: {data['scope']}")
    print("\nedit error rate by session date:")
    for day, st in data["edit_errors_by_date"].items():
        print(f"  {day}  calls={st['calls']:<5} err={st['err']:<4} ({st['rate_pct']}%)")
    print("\nexamples (sanitized: paths and tool error text only):")
    for klass, exs in data["examples"].items():
        print(f"  [{klass}]")
        for ex in exs:
            print(f"    {ex['session'][:32]} ({ex['origin']}/{ex['tool']}) {ex['path']}")
            if ex["snippet"]:
                print(f"      {ex['snippet']}")


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--root", default="~/.cozyphi", help="cozyphi home (default ~/.cozyphi)")
    parser.add_argument("--examples", type=int, default=2, help="examples per error class")
    parser.add_argument(
        "--debug-unknown", type=int, default=0, metavar="N",
        help="print first N unclassified results per run (terminal diagnostic; may contain transcript text)",
    )
    parser.add_argument("--json", action="store_true", help="dump raw JSON instead of text")
    args = parser.parse_args(argv)

    root = Path(args.root).expanduser()
    if not root.is_dir():
        print(f"error: {root} is not a directory", file=sys.stderr)
        return 2

    data = analyze(root, args.examples, args.debug_unknown)
    if args.debug_unknown and not args.json:
        for tool, samples in data["unknown_samples"].items():
            for s in samples:
                print(f"[unknown {tool}] {s['session'][:32]} {s['path']}", file=sys.stderr)
                print(f"  {s['snippet']}", file=sys.stderr)
    if args.json:
        json.dump(data, sys.stdout, indent=2, ensure_ascii=False)
        sys.stdout.write("\n")
    else:
        print_report(data)
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
