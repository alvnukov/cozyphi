#!/usr/bin/env python3
"""Analyze model edit/write mistakes in cozyphi session transcripts.

Walks cozyphi session transcripts (main sessions under <root>/session/ and
sub-agent jobs under <root>/jobs/<id>/session/, or any *.jsonl tree produced
by an eval runner), pairs tool calls with their role="tool" results,
classifies failures of edit/write/read(mode=edit)/grep and prints aggregate
frequencies, retry chains, per-cohort tables and sanitized examples.

Two vocabularies live side by side on purpose:

  * legacy classes   — the text heuristics this script has always used. They
    stay byte-identical so historical figures remain comparable, but they are
    NOT stable: a new typed refusal code changes which legacy bucket a failure
    lands in, which can manufacture "improvement" across harness versions.
  * stable categories — every legacy class name and every typed `[edit:<code>]`
    wire code is mapped onto one of a small, fixed set of categories. Any code
    without a mapping is reported at the top of the run instead of being
    silently dropped, so a new refusal code can never quietly vanish from a
    before/after comparison.

Only the Python standard library is used. The script never prints tool
arguments, file contents, or tool-result text. Reports contain aggregate
counts and call metadata only, so transcript secrets cannot reach the report.

For a reproducible model evaluation, put a ``manifest.json`` next to each
run's transcript. Its run-level fields are ``model``, ``model_version``,
``effort``, ``harness``, ``harness_revision``, ``scenario``, ``run``,
``task_success``, ``elapsed_ms``, ``usage`` and optional provider-reported
``cost_usd``. Missing fields remain explicitly unlabeled; the analyzer never
guesses a model version, revision, task result, or cost.

Usage:
    python3 scripts/analyze_edit_errors.py [--root ~/.cozyphi] [--examples N] [--json]
    python3 scripts/analyze_edit_errors.py --root OUT --baseline before.json
"""

from __future__ import annotations

import argparse
import json
import math
import posixpath
import re
import statistics
import sys
from collections import Counter
from collections.abc import Callable
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any

# Tools whose calls are paired with results. edit/write/read carry the legacy
# figures; grep and bash were added for trusted-observation and shell-fallback
# detection and never enter the legacy `classes` / `per_tool_errors` sections.
TRACKED_TOOLS = ("edit", "write", "read", "grep", "bash")

# The subset the legacy sections are computed over — do not extend.
LEGACY_TOOLS = ("edit", "write", "read")

# Success shapes, per tool, from the tool implementations:
#   read    -> "@read <path> (N lines, ...)"
#   edit    -> "@file <path>#TAG ..." plus "Re-read this file before another edit"
#   write   -> "wrote N bytes to <path>"
#   grep    -> "@file <path>#TAG" blocks when the ledger issued anchors
SUCCESS_SHAPES = {
    "read": (re.compile(r"^@file "),),  # mode=edit returns the @file path#TAG header
    "edit": (re.compile(r"^@file "),),
    "write": (re.compile(r"^wrote \d+ bytes to "),),
    "read_view": (re.compile(r"^@read "), re.compile(r"^@file "), re.compile(r"^\d+\|")),  # header formats changed over time
    "grep": (re.compile(r"^@file "), re.compile(r"^No matches found")),
}

CANCELED_TEXT = "User cancelled the tool call."

# Stable typed refusals start with "[edit:<code>]" (doc/edit-capability.md).
# One prefix maps to one class; matched before the legacy text heuristics so
# new transcripts stop guessing while old numbers stay comparable.
STABLE_EDIT_CODE = re.compile(r"^\[edit:([a-z_]+)\]")

# Successful edits report re-anchoring as a notice; a non-zero delta means the
# harness rebased the claimed range instead of the model hitting it exactly.
REBASE_NOTICE = re.compile(r"rebased edits\[\d+\] from \d+-\d+ to \d+-\d+ \(delta ([+-]?\d+)\)")

# New harnesses print this directly after the @file success header. It is the
# authoritative success classification; REBASE_NOTICE remains for old logs.
STABLE_EDIT_SUCCESS = re.compile(
    r"^@file \S+#[0-9A-Za-z]+[^\S\r\n]*\r?\n\[edit:(exact|rebased)\][^\S\r\n]*$",
    re.MULTILINE,
)

# A plan gate can repair an invalid step reference by binding the one eligible
# step. New transcripts name that recovery directly; old text remains useful
# for comparing the transition period.
STABLE_PLAN_AUTO_BOUND = re.compile(r"\[plan:auto_bound\]")
LEGACY_PLAN_AUTO_BOUND = re.compile(r"\bauto-bound to\b", re.IGNORECASE)

# "@file path#TAG" headers inside a grep/read result, used to decide whether a
# call was a trusted observation of a particular path.
FILE_HEADER = re.compile(r"^@file (\S+)#([0-9A-Za-z]+)\s*$", re.MULTILINE)

# Shell commands that look like a write to a file, for the (heuristic)
# shell-fallback signal.
SHELL_WRITE_TOKENS = (">", ">>", "sed -i", "tee", "perl -pi", "cat <<", "python")

# Ordered error classifiers: (class, predicate over the lowercased content).
# Order matters — the first match wins; keep specific patterns above generic.
ERROR_CLASSES: list[tuple[str, Callable[[str], bool]]] = [
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

# Stable categories. Every legacy class name and every typed `[edit:<code>]`
# code emitted by internal/tools/writetool/hashline.go and
# internal/tools/editledger/ledger.go maps here. A comparison across harness
# versions is only meaningful at this level: the wire code for "the model had
# no editable read" changed from free text to `[edit:no_capability]` and later
# split into snapshot_consumed / snapshot_evicted / anchor_not_observed, and
# all of those are one category.
STABLE_CATEGORIES: dict[str, str] = {
    # capability — the model never held (or already spent) a grant for this revision
    "no_capability": "capability",
    "snapshot_consumed": "capability",
    "snapshot_evicted": "capability",
    "anchor_not_observed": "capability",
    "mixed_grants": "capability",
    # stale — the model held a grant, but the file moved under it
    "stale_anchors": "stale",
    "tag_mismatch": "stale",
    "tag_changed": "stale",
    "changed_during_edit": "stale",
    "ambiguous_reanchor": "stale",
    # malformed — the call itself was not well formed
    "invalid_ref": "malformed",
    "missing_hash": "malformed",
    "missing_anchors": "malformed",
    "bad_range": "malformed",
    "range_inverted": "malformed",
    "out_of_bounds": "malformed",
    "overlap": "malformed",
    "missing_path": "malformed",
    "parse_args": "malformed",
    # gate — policy refused the call, the model's arguments were never judged
    "plan_gate": "gate",
    "permission": "gate",
    "withheld_retry": "gate",
    "outside_workspace": "gate",
    # harness — our side ran out of room or wiring
    "context_limit": "harness",
    "tool_not_found": "harness",
    "invalid_mode": "harness",
    "read_too_big": "harness",
    # io — the filesystem or an external command said no. shell_exit and
    # grep_failed are not edit failures; they are classified only so the tool
    # table stays honest, and they never enter the edit denominators.
    "write_failed": "io",
    "fs_error": "io",
    "shell_exit": "io",
    "grep_failed": "io",
    # terminal buckets
    "canceled": "canceled",
    "unclassified": "unclassified",
    "unknown": "unclassified",
}

# Outcome codes that are not failures; they exist in the Go enum but never
# reach a refusal string. Listed so the "every known code is mapped" test can
# tell "deliberately not an error" from "someone forgot a category".
NON_ERROR_CODES = frozenset({"granted"})

CATEGORY_ORDER = (
    "capability",
    "stale",
    "malformed",
    "gate",
    "harness",
    "io",
    "canceled",
    "unclassified",
    "uncategorized",
)

# Cohort tags: did the failure arrive as a stable typed code or as free text?
COHORT_TYPED = "typed"
COHORT_LEGACY = "legacy_text"

RETRY_KINDS = ("retry_unchanged", "retry_corrected_uninformed", "retry_informed")
SUCCESS_KINDS = ("exact", "rebased", "recovered")


@dataclass
class Call:
    """A tracked tool call paired with its result, if one was recorded."""

    session: Path
    origin: str  # "session" | "job"
    tool: str  # edit | write | read (mode=edit) | read_view | grep | bash
    args: dict[str, Any] = field(default_factory=dict)
    ts: str = ""
    path: str = ""
    cwd: str = ""
    status: str = "no_result"  # success | error | canceled | unknown | no_result
    klass: str = ""
    blind: bool = False  # legacy: edit retried on a path whose last edit failed without a re-read
    # Attribution
    model: str = "unknown"
    model_version: str = "unknown"
    effort: str = ""
    harness: str = "unlabeled"
    harness_revision: str = "unlabeled"
    scenario: str = "organic"
    # Wall time from the assistant entry (written after the model replied)
    # to its tool-result entry: tool execution, NOT model think time.
    tool_latency_ms: float | None = None
    # Outcome detail
    cohort_tag: str = ""  # typed | legacy_text, errors only
    category: str = ""
    retry_kind: str = ""
    success_kind: str = ""
    rebased: bool = False
    plan_auto_bound: bool = False
    observed_paths: tuple[str, ...] = ()  # @file headers seen in a successful result

    def cohort(self) -> tuple[str, str, str, str, str, str]:
        return (self.model, self.model_version, self.effort, self.harness, self.harness_revision, self.scenario)


CohortKey = tuple[str, str, str, str, str, str]


def manifest_cohort(manifest: dict[str, Any]) -> CohortKey:
    """Cohort for a run with no tool calls to supply entry-level attribution."""
    return (
        str(manifest.get("model") or "").strip() or "unknown",
        str(manifest.get("model_version") or "").strip() or "unknown",
        str(manifest.get("effort") or "").strip(),
        str(manifest.get("harness") or "").strip() or "unlabeled",
        str(manifest.get("harness_revision") or "").strip() or "unlabeled",
        str(manifest.get("scenario") or "").strip() or "organic",
    )


@dataclass(frozen=True)
class ResponseUsage:
    """One provider usage record, owned by its assistant response."""

    session: Path
    model: str
    model_version: str
    effort: str
    harness: str
    harness_revision: str
    scenario: str
    input_tokens: int
    output_tokens: int
    total_tokens: int

    def cohort(self) -> CohortKey:
        return (self.model, self.model_version, self.effort, self.harness, self.harness_revision, self.scenario)


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
    stable = STABLE_EDIT_CODE.match(content)
    if stable:
        return "error", stable.group(1)
    for klass, predicate in ERROR_CLASSES:
        if predicate(lowered):
            return "error", klass
    if tool == "bash":
        # bash prints arbitrary program output; only the harness' own exit
        # marker tells success from failure. The shell-fallback signal does not
        # depend on the outcome, so this only keeps the tool table honest.
        if "(exit error: exit status" in content:
            return "error", "shell_exit"
        return "success", ""
    if tool == "grep":
        # Any other grep result is a failure of the search itself. It is never
        # an edit attempt, so it stays out of the edit denominators.
        return "error", "grep_failed"
    return "unknown", ""


def category_of(klass: str) -> str:
    """Stable category for a legacy class name or a typed wire code."""
    return STABLE_CATEGORIES.get(klass, "uncategorized")


def normalize_path(value: str) -> str:
    value = value.strip().replace("\\", "/")
    while value.startswith("./"):
        value = value[2:]
    return value


def resolve_transcript_path(value: str, cwd: str = "") -> str:
    """Normalize a transcript path without guessing which directory owns it."""
    normalized = normalize_path(value)
    if not normalized or normalized.startswith("/"):
        return posixpath.normpath(normalized) if normalized else ""
    normalized_cwd = normalize_path(cwd)
    if normalized_cwd.startswith("/"):
        return posixpath.normpath(posixpath.join(normalized_cwd, normalized))
    return normalized


def canonical_path(value: str, known_paths: list[str], cwd: str = "") -> str:
    """Use one key for equivalent, cwd-resolved transcript path spellings."""
    normalized = resolve_transcript_path(value, cwd)
    if normalized in known_paths:
        return normalized
    if normalized:
        known_paths.append(normalized)
    return normalized


def path_matches(a: str, b: str, cwd: str = "") -> bool:
    """True when paths resolve identically in the transcript's session cwd.

    A suffix match is unsafe: ``/one/a.txt`` and ``/two/a.txt`` can both be
    shortened to ``a.txt``. Relative paths are therefore resolved only when
    the session supplied an absolute cwd; otherwise only an exact spelling
    matches.
    """
    resolved_a = resolve_transcript_path(a, cwd)
    resolved_b = resolve_transcript_path(b, cwd)
    return bool(resolved_a) and resolved_a == resolved_b


def normalized_edit_args(args: dict[str, Any], path: str | None = None) -> str:
    """Canonical form of an edit call, for "did the model retry it unchanged?"."""
    edits: list[dict[str, str | None]] = []
    raw = args.get("edits")
    if isinstance(raw, list):
        for item in raw:
            if not isinstance(item, dict):
                continue
            content = item.get("content")
            edits.append(
                {
                    "from": str(item.get("from") or "").strip(),
                    "to": str(item.get("to") or "").strip(),
                    # Omitted/null content deletes; an empty string is an
                    # explicit replacement and must not become the same retry.
                    "content": content if isinstance(content, str) else None,
                }
            )
    return json.dumps(
        {
            "path": path if path is not None else normalize_path(str(args.get("path") or "")),
            "hash": str(args.get("hash") or "").strip().upper(),
            "edits": edits,
        },
        sort_keys=True,
    )


def parse_ts(value: str) -> datetime | None:
    if not value:
        return None
    try:
        return datetime.fromisoformat(value)
    except ValueError:
        return None


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
    if files:
        return files
    # Eval output trees keep transcripts at <root>/<harness>/<model>/<scenario>/run-N/session/.
    for p in sorted(root.rglob("*.jsonl")):
        files.append((p, "job" if "jobs" in p.parts else "session"))
    return files


def load_manifest(path: Path, root: Path, cache: dict[Path, dict[str, Any]]) -> dict[str, Any]:
    """Nearest manifest.json at or above the transcript, up to root."""
    root_resolved = root.resolve()
    directory = path.parent.resolve()
    visited: list[Path] = []
    found: dict[str, Any] = {}
    while True:
        if directory in cache:
            found = cache[directory]
            break
        visited.append(directory)
        candidate = directory / "manifest.json"
        if candidate.is_file():
            try:
                raw = json.loads(candidate.read_text(encoding="utf-8"))
            except (OSError, json.JSONDecodeError):
                raw = {}
            found = raw if isinstance(raw, dict) else {}
            break
        if directory == root_resolved or directory.parent == directory:
            break
        directory = directory.parent
    for seen in visited:
        cache[seen] = found
    return found


def parse_session(
    path: Path,
    origin: str,
    manifest: dict[str, Any] | None = None,
) -> tuple[list[Call], list[ResponseUsage], int, int]:
    """Parse one transcript; returns calls, response usage, entries and bad lines."""
    manifest = manifest or {}
    manifest_model = str(manifest.get("model") or "").strip()
    manifest_model_version = str(manifest.get("model_version") or "").strip()
    manifest_effort = str(manifest.get("effort") or "").strip()
    harness = str(manifest.get("harness") or "").strip() or "unlabeled"
    harness_revision = str(manifest.get("harness_revision") or "").strip() or "unlabeled"
    scenario = str(manifest.get("scenario") or "").strip() or "organic"
    header_model = ""
    header_model_version = ""
    session_cwd = ""
    calls: dict[str, Call] = {}
    ordered: list[Call] = []
    response_usage: list[ResponseUsage] = []
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
            if entry.get("type") == "EntrySession":
                header_model = str(entry.get("model") or "").strip()
                header_model_version = str(entry.get("model_version") or "").strip()
                session_cwd = str(entry.get("cwd") or "").strip()
                continue
            if entry.get("type") != "EntryMessage":
                continue
            entries += 1
            msg = entry.get("message") or {}
            role = msg.get("role")
            if role == "assistant":
                model = str(entry.get("model") or "").strip() or header_model or manifest_model or "unknown"
                model_version = (
                    str(entry.get("model_version") or "").strip()
                    or header_model_version
                    or manifest_model_version
                    or "unknown"
                )
                effort = str(entry.get("effort") or "").strip() or manifest_effort
                usage = token_usage(entry.get("usage"))
                if usage is not None:
                    response_usage.append(
                        ResponseUsage(
                            session=path,
                            model=model,
                            model_version=model_version,
                            effort=effort,
                            harness=harness,
                            harness_revision=harness_revision,
                            scenario=scenario,
                            input_tokens=usage[0],
                            output_tokens=usage[1],
                            total_tokens=usage[2],
                        )
                    )
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
                        cwd=session_cwd,
                        model=model,
                        model_version=model_version,
                        effort=effort,
                        harness=harness,
                        harness_revision=harness_revision,
                        scenario=scenario,
                    )
                    calls[str(tc.get("id") or "")] = call
                    ordered.append(call)
            elif role == "tool":
                cid = str(msg.get("tool_call_id") or entry.get("tool_call_id") or "")
                pending_call = calls.get(cid)
                if pending_call is None or pending_call.status != "no_result":
                    continue  # unmatched id or a replayed result from a branch
                text = content_to_text(msg.get("content"))
                pending_call.status, pending_call.klass = classify(pending_call.tool, text)
                if pending_call.status == "error":
                    pending_call.cohort_tag = COHORT_TYPED if STABLE_EDIT_CODE.match(text) else COHORT_LEGACY
                if pending_call.status == "success":
                    pending_call.observed_paths = tuple(m.group(1) for m in FILE_HEADER.finditer(text))
                    pending_call.plan_auto_bound = bool(
                        STABLE_PLAN_AUTO_BOUND.search(text) or LEGACY_PLAN_AUTO_BOUND.search(text)
                    )
                    if pending_call.tool == "edit":
                        stable_success = STABLE_EDIT_SUCCESS.search(text)
                        if stable_success is not None:
                            pending_call.rebased = stable_success.group(1) == "rebased"
                        else:
                            deltas = [int(m.group(1)) for m in REBASE_NOTICE.finditer(text)]
                            pending_call.rebased = any(d != 0 for d in deltas)
                started = parse_ts(pending_call.ts)
                finished = parse_ts(str(entry.get("timestamp") or ""))
                if started is not None and finished is not None:
                    delta = (finished - started).total_seconds() * 1000.0
                    if delta >= 0:
                        pending_call.tool_latency_ms = delta
    return ordered, response_usage, entries, bad


def group_by_session(calls: list[Call]) -> dict[Path, list[Call]]:
    out: dict[Path, list[Call]] = {}
    for c in calls:
        out.setdefault(c.session, []).append(c)
    return out


def analyze_chains(calls: list[Call]) -> list[dict[str, Any]]:
    """Legacy retry chains, kept byte-identical for historical comparison.

    Known to over-count: any edit following a failed edit on the same path is
    "blind" unless a read call intervened, even when the retry was corrected
    and succeeded, and any read (including a failed one) clears the flag. The
    honest split lives in analyze_retries(); this function only feeds
    `chains.blind_retries`, reported as `legacy_blind_retries`.
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
                    "cohort": group[0].cohort(),
                }
            )
    return chains


def is_trusted_observation(call: Call, path: str) -> bool:
    """Did this call give the model fresh, usable anchors for `path`?

    Only successful calls count. A failed read or grep observed nothing, and a
    view-mode read carries no LINE#HASH anchors at all.
    """
    if call.status != "success":
        return False
    if call.tool in ("read", "write", "edit"):
        # A legacy success shape may lack the returned @file anchor block.
        # The argument alone is not evidence that the model received anchors.
        return bool(call.observed_paths) and bool(call.path) and path_matches(call.path, path, call.cwd)
    if call.tool == "grep":
        return any(path_matches(header, path, call.cwd) for header in call.observed_paths)
    return False


def looks_like_shell_write(command: str, path: str) -> bool:
    """Heuristic: does this shell command write the file the edit failed on?"""
    base = normalize_path(path).rsplit("/", 1)[-1]
    if not base or base not in command:
        return False
    return any(token in command for token in SHELL_WRITE_TOKENS)


def analyze_retries(calls: list[Call]) -> dict[str, Any]:
    """Chronological per (session, path) walk: retries, success kinds, fallbacks.

    Annotates each edit Call with retry_kind / success_kind and returns the
    aggregate counters. A retry is only "unchanged" when the normalized call
    (path, hash, edits) is identical to the failed one — a corrected retry is
    a different, and much better, thing.
    """
    totals: Counter[str] = Counter()
    for evs in group_by_session(calls).values():
        pending: dict[str, Call] = {}  # path -> most recent failed edit
        trusted: dict[str, bool] = {}  # path -> observation since that failure
        edited_ok: set[str] = set()  # paths with an earlier successful edit
        known_paths: list[str] = []
        for call in evs:
            path = canonical_path(call.path, known_paths, call.cwd)
            if call.tool == "bash":
                command = str(call.args.get("command") or "")
                for failed_path in list(pending):
                    if looks_like_shell_write(command, failed_path):
                        call.retry_kind = "shell_fallback"
                        totals["shell_fallback"] += 1
                        break
                continue
            if not path:
                for observed in list(pending):
                    if is_trusted_observation(call, observed):
                        trusted[observed] = True
                continue
            if call.tool == "write":
                if path in pending:
                    call.retry_kind = "edit_fail_then_write"
                    totals["edit_fail_then_write"] += 1
                elif path in edited_ok:
                    call.retry_kind = "write_after_edit_ok"
                    totals["write_after_edit_ok"] += 1
            if call.tool == "edit":
                previous = pending.get(path)
                if previous is not None:
                    if normalized_edit_args(call.args, path) == normalized_edit_args(previous.args, path):
                        call.retry_kind = "retry_unchanged"
                    elif trusted.get(path):
                        call.retry_kind = "retry_informed"
                    else:
                        call.retry_kind = "retry_corrected_uninformed"
                    totals[call.retry_kind] += 1
                if call.status == "error":
                    pending[path] = call
                    trusted[path] = False
                elif call.status == "success":
                    if previous is not None:
                        call.success_kind = "recovered"
                    elif call.rebased:
                        call.success_kind = "rebased"
                    else:
                        call.success_kind = "exact"
                    totals[call.success_kind] += 1
                    pending.pop(path, None)
                    edited_ok.add(path)
            # Any successful observation of this path clears the "uninformed" flag.
            for observed in list(pending):
                if is_trusted_observation(call, observed):
                    trusted[observed] = True
    return {kind: totals.get(kind, 0) for kind in (*RETRY_KINDS, *SUCCESS_KINDS, "edit_fail_then_write", "write_after_edit_ok", "shell_fallback")}


def scope_of(path: str) -> str:
    if ".worktrees/" in path:
        return "worktree"
    if path.startswith(("/", "~")):
        return "absolute"
    return "cwd-relative"


def percentile(values: list[float], pct: float) -> float:
    if not values:
        return 0.0
    ordered = sorted(values)
    if len(ordered) == 1:
        return ordered[0]
    position = (len(ordered) - 1) * pct
    low, high = math.floor(position), math.ceil(position)
    if low == high:
        return ordered[int(position)]
    return ordered[low] * (high - position) + ordered[high] * (position - low)


def numeric(value: object) -> float | None:
    """A finite numeric manifest value, excluding bool and malformed input."""
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return None
    result = float(value)
    return result if math.isfinite(result) else None


def token_usage(value: object) -> tuple[int, int, int] | None:
    """Read provider token totals, accepting the harness's legacy aliases."""
    if not isinstance(value, dict):
        return None

    def count(primary: str, legacy: str = "") -> int | None:
        raw = value.get(primary)
        if raw is None and legacy:
            raw = value.get(legacy)
        parsed = numeric(raw)
        return int(parsed) if parsed is not None else None

    input_tokens = count("input_tokens", "prompt_tokens")
    output_tokens = count("output_tokens", "completion_tokens")
    total_tokens = count("total_tokens")
    if input_tokens is None and output_tokens is None and total_tokens is None:
        return None
    input_tokens = input_tokens or 0
    output_tokens = output_tokens or 0
    return input_tokens, output_tokens, total_tokens if total_tokens is not None else input_tokens + output_tokens


def cohort_label(key: tuple[str, str, str, str, str, str]) -> str:
    model, model_version, effort, harness, harness_revision, scenario = key
    return f"{model}@{model_version}|{effort or '-'}|{harness}@{harness_revision}|{scenario}"


def build_cohorts(
    calls: list[Call],
    chains: list[dict[str, Any]],
    manifests: dict[Path, dict[str, Any]],
    response_usage: list[ResponseUsage],
) -> list[dict[str, Any]]:
    """Per (model, version, effort, harness, revision, scenario) table."""
    keys: dict[CohortKey, list[Call]] = {}
    session_keys: dict[Path, set[CohortKey]] = {}
    for call in calls:
        key = call.cohort()
        keys.setdefault(key, []).append(call)
        session_keys.setdefault(call.session, set()).add(key)

    usage_by_cohort: dict[CohortKey, list[ResponseUsage]] = {}
    for usage in response_usage:
        key = usage.cohort()
        usage_by_cohort.setdefault(key, []).append(usage)
        session_keys.setdefault(usage.session, set()).add(key)

    # A run can complete before the model calls a tool. Include its manifest in
    # the model cohort so zero-call failures remain visible in evaluation data.
    for session, manifest in manifests.items():
        if session not in session_keys:
            key = manifest_cohort(manifest)
            keys.setdefault(key, [])
            session_keys[session] = {key}

    chains_by_cohort: dict[CohortKey, list[dict[str, Any]]] = {}
    for chain in chains:
        key = chain["cohort"]
        chains_by_cohort.setdefault(key, []).append(chain)

    rows: list[dict[str, Any]] = []
    for key, group in keys.items():
        sessions = {session for session, cohort_keys in session_keys.items() if key in cohort_keys}
        edits = [c for c in group if c.tool == "edit"]
        errors = [c for c in edits if c.status == "error"]
        latencies = [c.tool_latency_ms for c in edits if c.tool_latency_ms is not None]
        categories: Counter[str] = Counter()
        for c in group:
            if c.tool != "edit":
                continue
            if c.status == "canceled":
                categories["canceled"] += 1
            elif c.status == "error":
                categories[category_of(c.klass or "unclassified")] += 1
        cohort_chains = chains_by_cohort.get(key, [])
        run_manifests = [manifests[s] for s in sessions if s in manifests]
        graded = [
            manifests[s]
            for s in sessions
            if s in manifests and ("task_success" in manifests[s] or "correct" in manifests[s])
        ]
        elapsed = [numeric(m.get("elapsed_ms", m.get("wall_ms"))) for m in run_manifests]
        costs = [numeric(m.get("cost_usd")) for m in run_manifests]
        usage_runs: list[tuple[int, int, int]] = []
        for session in sessions:
            manifest_usage = token_usage(manifests[session].get("usage")) if session in manifests else None
            if manifest_usage is not None:
                usage_runs.append(manifest_usage)
                continue
            transcript_usage = [u for u in usage_by_cohort.get(key, []) if u.session == session]
            if transcript_usage:
                usage_runs.append(
                    (
                        sum(u.input_tokens for u in transcript_usage),
                        sum(u.output_tokens for u in transcript_usage),
                        sum(u.total_tokens for u in transcript_usage),
                    )
                )
        input_tokens = sum(usage[0] for usage in usage_runs)
        output_tokens = sum(usage[1] for usage in usage_runs)
        total_tokens = sum(usage[2] for usage in usage_runs)
        rows.append(
            {
                "cohort": cohort_label(key),
                "model": key[0],
                "model_version": key[1],
                "effort": key[2],
                "harness": key[3],
                "harness_revision": key[4],
                "scenario": key[5],
                "sessions": len(sessions),
                "edit_attempts": len(edits),
                "edit_errors": len(errors),
                "edit_error_rate_pct": round(100.0 * len(errors) / len(edits), 1) if edits else 0.0,
                "categories": dict(categories.most_common()),
                "chains": len(cohort_chains),
                "first_try_success": sum(1 for ch in cohort_chains if ch["attempts"] == 1 and ch["succeeded"]),
                "retries": {kind: sum(1 for c in edits if c.retry_kind == kind) for kind in RETRY_KINDS},
                "success_kinds": {kind: sum(1 for c in edits if c.success_kind == kind) for kind in SUCCESS_KINDS},
                "edit_fail_then_write": sum(1 for c in group if c.tool == "write" and c.retry_kind == "edit_fail_then_write"),
                "correct": sum(1 for m in graded if m.get("task_success", m.get("correct"))),
                "graded": len(graded),
                "run_elapsed_ms": round(sum(v for v in elapsed if v is not None), 1),
                "reported_cost_usd": (
                    round(sum(v for v in costs if v is not None), 6) if any(v is not None for v in costs) else None
                ),
                "cost_runs": sum(1 for v in costs if v is not None),
                "reported_input_tokens": input_tokens,
                "reported_output_tokens": output_tokens,
                "reported_total_tokens": total_tokens,
                "usage_runs": len(usage_runs),
                "input_tokens": input_tokens,
                "output_tokens": output_tokens,
                "tool_latency_ms_median": round(statistics.median(latencies), 1) if latencies else 0.0,
                "tool_latency_ms_p90": round(percentile(latencies, 0.9), 1) if latencies else 0.0,
                "plan_auto_bound": sum(1 for c in group if c.plan_auto_bound),
            }
        )
    rows.sort(key=lambda r: (-int(r["edit_attempts"]), str(r["cohort"])))
    return rows


def analyze(root: Path, examples: int, debug_unknown: int = 0) -> dict[str, Any]:
    files = session_files(root)
    all_calls: list[Call] = []
    all_response_usage: list[ResponseUsage] = []
    entries = bad = 0
    n_sessions = n_jobs = 0
    manifest_cache: dict[Path, dict[str, Any]] = {}
    manifests: dict[Path, dict[str, Any]] = {}
    for path, origin in files:
        if origin == "job":
            n_jobs += 1
        else:
            n_sessions += 1
        manifest = load_manifest(path, root, manifest_cache)
        if manifest:
            manifests[path] = manifest
        calls, response_usage, e, b = parse_session(path, origin, manifest)
        entries += e
        bad += b
        all_calls.extend(calls)
        all_response_usage.extend(response_usage)

    by_tool: dict[str, list[Call]] = {}
    for c in all_calls:
        by_tool.setdefault(c.tool, []).append(c)

    class_counts: Counter[str] = Counter()
    per_tool_errors: dict[str, Counter[str]] = {}
    examples_by_class: dict[str, list[Call]] = {}
    category_counts: Counter[str] = Counter()
    category_by_cohort_tag: dict[str, Counter[str]] = {COHORT_TYPED: Counter(), COHORT_LEGACY: Counter()}
    uncategorized: Counter[str] = Counter()
    # For anchor errors: was the failing call a single edit or a multi-edit batch?
    # Counts only (len of the edits array), never edit contents.
    anchor_batch = {"single": 0, "multi": 0}
    for tool in LEGACY_TOOLS:
        for c in by_tool.get(tool, []):
            if c.status == "canceled":
                class_counts["canceled"] += 1
                per_tool_errors.setdefault(tool, Counter())["canceled"] += 1
                if c.tool == "edit":
                    c.category = "canceled"
                    category_counts["canceled"] += 1
                    category_by_cohort_tag[COHORT_LEGACY]["canceled"] += 1
            elif c.status == "error":
                klass = c.klass or "unclassified"
                class_counts[klass] += 1
                per_tool_errors.setdefault(tool, Counter())[klass] += 1
                examples_by_class.setdefault(klass, []).append(c)
                if c.tool == "edit":
                    category = category_of(klass)
                    c.category = category
                    category_counts[category] += 1
                    category_by_cohort_tag[c.cohort_tag or COHORT_LEGACY][category] += 1
                    if category == "uncategorized":
                        uncategorized[klass] += 1
                if klass in ("stale_anchors", "tag_mismatch", "no_capability") and c.tool == "edit":
                    edits = c.args.get("edits")
                    anchor_batch["multi" if isinstance(edits, list) and len(edits) > 1 else "single"] += 1

    chains = analyze_chains(all_calls)
    retries = analyze_retries(all_calls)

    total_chains = len(chains)
    first_try = sum(1 for ch in chains if ch["attempts"] == 1 and ch["succeeded"])
    retried = sum(1 for ch in chains if ch["attempts"] > 1)
    blind_total = sum(ch["blind_retries"] for ch in chains)

    # Legacy fallback approximation: paths where an edit failed and a write to
    # the same path also happened in the same session (order NOT checked).
    edit_fail_then_write = 0
    write_after_edit_ok = 0
    for group in group_by_session(all_calls).values():
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

    edit_attempts = len(by_tool.get("edit", []))
    cohorts = build_cohorts(all_calls, chains, manifests, all_response_usage)

    return {
        "root": str(root),
        "corpus": {
            "files": len(files),
            "sessions": n_sessions,
            "jobs": n_jobs,
            "entries": entries,
            "bad_lines": bad,
            "manifests": len(manifests),
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
        "categories": {
            "denominator": edit_attempts,
            "totals": dict(category_counts.most_common()),
            "by_cohort_tag": {tag: dict(counter.most_common()) for tag, counter in category_by_cohort_tag.items()},
        },
        "uncategorized_codes": dict(uncategorized.most_common()),
        "chains": {
            "total": total_chains,
            "first_try_success": first_try,
            "retried": retried,
            "blind_retries": blind_total,
            "worst": [
                {k: v for k, v in ch.items() if k != "cohort"}
                for ch in sorted(chains, key=lambda ch: (-int(ch["errors"]), -int(ch["attempts"])))[:10]
            ],
        },
        "retries": {
            "denominator": edit_attempts,
            "retry_unchanged": retries["retry_unchanged"],
            "retry_corrected_uninformed": retries["retry_corrected_uninformed"],
            "retry_informed": retries["retry_informed"],
            "legacy_blind_retries": blind_total,
        },
        "success_kinds": {kind: retries[kind] for kind in SUCCESS_KINDS},
        "edit_outcomes": {
            **{kind: retries[kind] for kind in SUCCESS_KINDS},
            "refused": sum(1 for c in by_tool.get("edit", []) if c.status == "error"),
        },
        "plan_auto_bound": sum(1 for c in all_calls if c.plan_auto_bound),
        "fallbacks": {
            "edit_fail_then_write": retries["edit_fail_then_write"],
            "write_after_edit_ok": retries["write_after_edit_ok"],
            "shell_fallback": retries["shell_fallback"],
        },
        "cohorts": cohorts,
        "edit_fail_then_write": edit_fail_then_write,
        "write_after_edit_ok": write_after_edit_ok,
        "scope": dict(scope),
        "edit_errors_by_date": timeline,
        "anchor_batch": anchor_batch,
        "unknown_samples": {
            tool: [
                {"session": c.session.name, "path": c.path}
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
                }
                for c in calls[:examples]
            ]
            for klass, calls in sorted(examples_by_class.items())
        },
    }


def rate(n: int, denominator: int) -> str:
    if denominator <= 0:
        return f"{n}/0"
    return f"{n}/{denominator} ({100.0 * n / denominator:.1f}%)"


def print_report(data: dict[str, Any]) -> None:
    corpus = data["corpus"]
    uncategorized = data.get("uncategorized_codes") or {}
    if uncategorized:
        print("!! codes with no stable category (add them to STABLE_CATEGORIES):")
        for code, count in uncategorized.items():
            print(f"   {code}  x{count}")
        print()
    print(f"corpus: {data['root']}")
    print(
        f"  files={corpus['files']} (sessions={corpus['sessions']}, jobs={corpus['jobs']}), "
        f"entries={corpus['entries']}, bad_lines={corpus['bad_lines']}, manifests={corpus.get('manifests', 0)}"
    )
    print("\ntool outcomes:")
    for tool, st in data["tools"].items():
        total, err = st["calls"], st["error"]
        err_rate = (100.0 * err / total) if total else 0.0
        print(
            f"  {tool:<10} calls={total:<5} ok={st['success']:<5} err={err:<4} "
            f"({err_rate:.1f}%) canceled={st['canceled']} unknown={st['unknown']} no_result={st['no_result']}"
        )
    cats = data["categories"]
    denominator = int(cats["denominator"])
    print(f"\nstable categories (edit failures; rate over {denominator} edit attempts):")
    typed = cats["by_cohort_tag"].get(COHORT_TYPED, {})
    legacy = cats["by_cohort_tag"].get(COHORT_LEGACY, {})
    for category in CATEGORY_ORDER:
        count = int(cats["totals"].get(category, 0))
        if not count:
            continue
        print(
            f"  {category:<14} {rate(count, denominator):<20} "
            f"typed={typed.get(category, 0)} legacy_text={legacy.get(category, 0)}"
        )
    print("\nlegacy error classes (unstable across harness versions; kept for history):")
    total_err = sum(data["classes"].values()) or 1
    for klass, count in data["classes"].items():
        print(f"  {klass:<20} {count:<5} ({100.0 * count / total_err:.1f}%)  -> {category_of(klass)}")
    print("\nerror classes per tool:")
    for tool, classes in data["per_tool_errors"].items():
        print(f"  {tool}: {classes}")
    chains = data["chains"]
    print(f"\nanchor errors by edits-in-call (stale/mismatch/no_capability): {data['anchor_batch']}")
    print("\nretry chains (edit, per path per session):")
    print(
        f"  chains={chains['total']} first_try_success={chains['first_try_success']} "
        f"retried={chains['retried']}"
    )
    retries = data["retries"]
    print(f"  retry_unchanged            {rate(int(retries['retry_unchanged']), denominator)}")
    print(f"  retry_corrected_uninformed {rate(int(retries['retry_corrected_uninformed']), denominator)}")
    print(f"  retry_informed             {rate(int(retries['retry_informed']), denominator)}")
    print(
        f"  legacy_blind_retries       {rate(int(retries['legacy_blind_retries']), denominator)}"
        "  [legacy definition: counted corrected retries as blind]"
    )
    kinds = data["edit_outcomes"]
    print(
        f"\nedit outcomes: exact={kinds['exact']} rebased={kinds['rebased']} "
        f"recovered={kinds['recovered']} refused={kinds['refused']}"
        "  (precedence: recovered > rebased > exact)"
    )
    print(f"plan auto-bound recoveries: {data['plan_auto_bound']}")
    fallbacks = data["fallbacks"]
    print("\nfallbacks (chronological, within one session):")
    print(f"  edit_fail_then_write  {fallbacks['edit_fail_then_write']}")
    print(f"  write_after_edit_ok   {fallbacks['write_after_edit_ok']}")
    print(f"  shell_fallback        {fallbacks['shell_fallback']}  [heuristic: bash command naming the file plus a write token]")
    print(
        f"  legacy (set intersection, order ignored): edit_fail_then_write={data['edit_fail_then_write']} "
        f"write_after_edit_ok={data['write_after_edit_ok']}"
    )
    print("\nworst (session, path) chains by edit errors (blind = legacy count):")
    for ch in chains["worst"]:
        if ch["errors"] == 0:
            continue
        print(
            f"    {ch['errors']} err / {ch['attempts']} tries  blind={ch['blind_retries']}  "
            f"{ch['session'][:28]}  {ch['path']}"
        )
    print_cohorts(data["cohorts"])
    print(f"\nedit/write path scope: {data['scope']}")
    print("\nedit error rate by session date:")
    for day, st in data["edit_errors_by_date"].items():
        print(f"  {day}  calls={st['calls']:<5} err={st['err']:<4} ({st['rate_pct']}%)")
    print("\nexamples (metadata only; tool-result text is never emitted):")
    for klass, exs in data["examples"].items():
        print(f"  [{klass}]")
        for ex in exs:
            print(f"    {ex['session'][:32]} ({ex['origin']}/{ex['tool']}) {ex['path']}")


def print_cohorts(cohorts: list[dict[str, Any]]) -> None:
    print("\ncohorts (model@version | effort | harness@revision | scenario):")
    if not cohorts:
        print("  (none)")
        return
    for row in cohorts:
        print(f"  {row['cohort']}")
        print(
            f"    sessions={row['sessions']} edits={row['edit_attempts']} "
            f"errors={rate(int(row['edit_errors']), int(row['edit_attempts']))} "
            f"chains={row['chains']} first_try={row['first_try_success']}"
        )
        print(f"    categories={row['categories'] or '{}'}")
        print(
            f"    retries={row['retries']} success={row['success_kinds']} "
            f"edit_fail_then_write={row['edit_fail_then_write']}"
        )
        graded = int(row["graded"])
        correct = f"{row['correct']}/{graded}" if graded else "n/a (no manifest)"
        cost = row["reported_cost_usd"] if row["cost_runs"] else "n/a (not reported)"
        print(
            f"    correct={correct} runs_elapsed_ms={row['run_elapsed_ms']} cost_usd={cost} "
            f"reported_tokens={row['reported_input_tokens']}/{row['reported_output_tokens']}/"
            f"{row['reported_total_tokens']} (runs={row['usage_runs']})"
        )
        print(
            f"    plan_auto_bound={row['plan_auto_bound']} tool_latency_ms median={row['tool_latency_ms_median']} "
            f"p90={row['tool_latency_ms_p90']}"
        )


def print_baseline_diff(current: dict[str, Any], baseline: dict[str, Any]) -> None:
    """Before/after deltas at the stable-category level, with denominators."""
    print("\n=== baseline comparison ===")
    print(f"baseline root: {baseline.get('root', '?')}")
    print(f"current  root: {current.get('root', '?')}")
    before_n = int(baseline.get("categories", {}).get("denominator", 0))
    after_n = int(current.get("categories", {}).get("denominator", 0))
    print(f"edit attempts: {before_n} -> {after_n}")
    print("\ncategories (n/N over edit attempts):")
    before_cats = baseline.get("categories", {}).get("totals", {})
    after_cats = current.get("categories", {}).get("totals", {})
    for category in CATEGORY_ORDER:
        before = int(before_cats.get(category, 0))
        after = int(after_cats.get(category, 0))
        if not before and not after:
            continue
        before_pct = 100.0 * before / before_n if before_n else 0.0
        after_pct = 100.0 * after / after_n if after_n else 0.0
        print(
            f"  {category:<14} {before}/{before_n} ({before_pct:.1f}%) -> "
            f"{after}/{after_n} ({after_pct:.1f}%)  delta {after_pct - before_pct:+.1f}pp"
        )
    print("\nretries (n/N over edit attempts):")
    before_retries = baseline.get("retries", {})
    after_retries = current.get("retries", {})
    for kind in (*RETRY_KINDS, "legacy_blind_retries"):
        before = int(before_retries.get(kind, 0))
        after = int(after_retries.get(kind, 0))
        before_pct = 100.0 * before / before_n if before_n else 0.0
        after_pct = 100.0 * after / after_n if after_n else 0.0
        print(
            f"  {kind:<28} {before}/{before_n} ({before_pct:.1f}%) -> "
            f"{after}/{after_n} ({after_pct:.1f}%)  delta {after_pct - before_pct:+.1f}pp"
        )
    print("\nchains:")
    for key in ("total", "first_try_success", "retried"):
        print(f"  {key:<20} {baseline.get('chains', {}).get(key, 0)} -> {current.get('chains', {}).get(key, 0)}")
    print("\ncohorts:")
    before_rows = {str(r["cohort"]): r for r in baseline.get("cohorts", [])}
    after_rows = {str(r["cohort"]): r for r in current.get("cohorts", [])}
    for label in sorted(set(before_rows) | set(after_rows)):
        before_row = before_rows.get(label)
        after_row = after_rows.get(label)
        before_txt = (
            rate(int(before_row["edit_errors"]), int(before_row["edit_attempts"])) if before_row else "absent"
        )
        after_txt = rate(int(after_row["edit_errors"]), int(after_row["edit_attempts"])) if after_row else "absent"
        print(f"  {label}\n    edit errors {before_txt} -> {after_txt}")


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0] if __doc__ else "")
    parser.add_argument("--root", default="~/.cozyphi", help="cozyphi home (default ~/.cozyphi)")
    parser.add_argument("--examples", type=int, default=2, help="examples per error class")
    parser.add_argument(
        "--debug-unknown", type=int, default=0, metavar="N",
        help="print metadata for the first N unclassified results per run",
    )
    parser.add_argument("--json", action="store_true", help="dump raw JSON instead of text")
    parser.add_argument(
        "--baseline", metavar="FILE", default="",
        help="a report saved with --json; print before/after deltas with denominators",
    )
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
    if args.json:
        json.dump(data, sys.stdout, indent=2, ensure_ascii=False)
        sys.stdout.write("\n")
    else:
        print_report(data)
    if args.baseline:
        baseline_path = Path(args.baseline).expanduser()
        try:
            raw = json.loads(baseline_path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as exc:
            print(f"error: cannot read baseline {baseline_path}: {exc}", file=sys.stderr)
            return 2
        if not isinstance(raw, dict):
            print(f"error: baseline {baseline_path} is not a report object", file=sys.stderr)
            return 2
        print_baseline_diff(data, raw)
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
