#!/usr/bin/env python3
"""Regression tests for scripts/analyze_edit_errors.py.

Every test builds synthetic transcripts in a temp directory and runs the real
analyzer over them, so the assertions are about the shipped classifier and not
about a stand-in. The transcript shape mirrors internal/session/entry.go:
an optional EntrySession header followed by EntryMessage entries carrying
OpenAI-shaped assistant messages and role="tool" results.

Run:
    python3 -m unittest discover -s scripts -p 'test_*.py'
"""

from __future__ import annotations

import io
import json
import re
import sys
import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any

sys.path.insert(0, str(Path(__file__).resolve().parent))

import analyze_edit_errors as aee

REPO_ROOT = Path(__file__).resolve().parents[1]
BASE_TIME = datetime(2026, 9, 5, 10, 0, 0, tzinfo=timezone.utc)

# Typed codes the Go harness can emit today. The test also scrapes the Go
# sources so a newly added code fails here instead of silently landing in
# "uncategorized" months later; this list keeps the test meaningful if the
# files ever move.
KNOWN_TYPED_CODES = frozenset(
    {
        "ambiguous_reanchor",
        "anchor_not_observed",
        "changed_during_edit",
        "granted",
        "invalid_ref",
        "mixed_grants",
        "no_capability",
        "out_of_bounds",
        "overlap",
        "range_inverted",
        "snapshot_consumed",
        "snapshot_evicted",
        "tag_changed",
        "unknown",
    }
)


def edit_args(
    path: str,
    file_hash: str = "AB12",
    frm: str = "3#DEAD",
    to: str = "4#BEEF",
    content: str = "new line",
) -> dict[str, Any]:
    return {
        "path": path,
        "hash": file_hash,
        "edits": [{"from": frm, "to": to, "content": content}],
    }


def edit_ok(path: str, tag: str = "CD34", notice: str = "") -> str:
    """A successful edit result: header, optional notices, successor anchors."""
    body = f"@file {path}#{tag}\n"
    if notice:
        body += notice + "\n"
    return body + "@edit successor\n3#F00D|new line\n"


def edit_refusal(code: str) -> str:
    return (
        f"[edit:{code}] the claimed anchors do not describe the current file. "
        "Do not retry the same call unchanged. Re-read the file with mode=\"edit\"."
    )


def read_edit_ok(path: str, tag: str = "AB12") -> str:
    return f"@file {path}#{tag}\n1#C0DE|first\n2#DEAD|second\n"


def write_ok(path: str, tag: str = "EF56") -> str:
    return f"wrote 42 bytes to {path}\n\n@file {path}#{tag}\n1#C0DE|first\n"


def grep_ok(display_path: str, tag: str = "AB12") -> str:
    return f"@file {display_path}#{tag}\n3#F00D|match here\n"


class Builder:
    """Assembles one synthetic transcript file."""

    def __init__(
        self, *, header_model: str = "", default_model: str = "m1", effort: str = "", cwd: str = ""
    ) -> None:
        self.lines: list[str] = []
        self.default_model = default_model
        self.effort = effort
        self.clock_ms = 0
        self.seq = 0
        if header_model or cwd:
            header: dict[str, Any] = {
                "type": "EntrySession",
                "id": "s1",
                "timestamp": "2026-09-05T10-00-00",
            }
            if cwd:
                header["cwd"] = cwd
            if header_model:
                header["model"] = header_model
            self.lines.append(
                json.dumps(header)
            )

    def _ts(self, ms: int) -> str:
        return (BASE_TIME + timedelta(milliseconds=ms)).isoformat()

    def batch(
        self,
        items: list[tuple[str, dict[str, Any], str]],
        *,
        model: str | None = None,
        effort: str | None = None,
        usage: dict[str, int] | None = None,
        latency_ms: int = 1000,
    ) -> list[str]:
        """One assistant message with N tool calls, then their N results."""
        ids: list[str] = []
        tool_calls: list[dict[str, Any]] = []
        for tool, args, _ in items:
            self.seq += 1
            cid = f"call_{self.seq}"
            ids.append(cid)
            tool_calls.append(
                {
                    "id": cid,
                    "type": "function",
                    "function": {"name": tool, "arguments": json.dumps(args)},
                }
            )
        entry: dict[str, Any] = {
            "type": "EntryMessage",
            "id": f"a{self.seq}",
            "timestamp": self._ts(self.clock_ms),
            "message": {"role": "assistant", "content": "", "tool_calls": tool_calls},
        }
        chosen = self.default_model if model is None else model
        if chosen:
            entry["model"] = chosen
        chosen_effort = self.effort if effort is None else effort
        if chosen_effort:
            entry["effort"] = chosen_effort
        if usage:
            entry["usage"] = usage
        self.lines.append(json.dumps(entry))
        self.clock_ms += latency_ms
        for cid, (_, _, result) in zip(ids, items, strict=True):
            self.lines.append(
                json.dumps(
                    {
                        "type": "EntryMessage",
                        "id": f"t{cid}",
                        "timestamp": self._ts(self.clock_ms),
                        "message": {"role": "tool", "tool_call_id": cid, "content": result},
                    }
                )
            )
            self.clock_ms += 10
        return ids

    def call(
        self,
        tool: str,
        args: dict[str, Any],
        result: str,
        *,
        model: str | None = None,
        effort: str | None = None,
        usage: dict[str, int] | None = None,
        latency_ms: int = 1000,
    ) -> str:
        return self.batch(
            [(tool, args, result)], model=model, effort=effort, usage=usage, latency_ms=latency_ms
        )[0]

    def write(self, root: Path, name: str = "2026-09-05-abc.jsonl", subdir: str = "session") -> Path:
        directory = root / subdir if subdir else root
        directory.mkdir(parents=True, exist_ok=True)
        target = directory / name
        target.write_text("\n".join(self.lines) + "\n", encoding="utf-8")
        return target


class AnalyzerTestCase(unittest.TestCase):
    """Shared temp root plus a helper that runs the real analyzer."""

    def setUp(self) -> None:
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.root = Path(self._tmp.name)

    def analyze(self) -> dict[str, Any]:
        return aee.analyze(self.root, 2)


class RetryClassificationTest(AnalyzerTestCase):
    def test_empty_replacement_is_not_the_same_as_a_deletion(self) -> None:
        deleted = edit_args("/w/a.txt")
        deleted["edits"][0].pop("content")
        empty_replacement = edit_args("/w/a.txt", content="")

        self.assertNotEqual(
            aee.normalized_edit_args(deleted),
            aee.normalized_edit_args(empty_replacement),
        )

    def test_corrected_retry_without_read_is_not_blind(self) -> None:
        """invalid_ref, then a corrected edit that succeeds without a re-read.

        The legacy metric called this "blind"; it is a legal recovery path.
        """
        b = Builder()
        b.call("edit", edit_args("/w/a.txt", frm="3#DEAD"), edit_refusal("invalid_ref"))
        b.call("edit", edit_args("/w/a.txt", frm="4#DEAD"), edit_ok("a.txt"))
        b.write(self.root)

        data = self.analyze()
        retries = data["retries"]
        self.assertEqual(retries["retry_corrected_uninformed"], 1)
        self.assertEqual(retries["retry_unchanged"], 0)
        self.assertEqual(retries["retry_informed"], 0)
        self.assertEqual(retries["denominator"], 2)
        # The legacy number is kept, and it disagrees on purpose.
        self.assertEqual(retries["legacy_blind_retries"], 1)
        self.assertEqual(data["success_kinds"], {"exact": 0, "rebased": 0, "recovered": 1})

    def test_unchanged_recall_is_retry_unchanged(self) -> None:
        b = Builder()
        args = edit_args("/w/a.txt")
        b.call("edit", args, edit_refusal("stale_anchors"))
        b.call("edit", dict(args), edit_refusal("stale_anchors"))
        b.write(self.root)

        retries = self.analyze()["retries"]
        self.assertEqual(retries["retry_unchanged"], 1)
        self.assertEqual(retries["retry_corrected_uninformed"], 0)

    def test_unchanged_recall_ignores_key_order(self) -> None:
        b = Builder()
        b.call("edit", {"path": "/w/a.txt", "hash": "AB12", "edits": [{"from": "3#D", "to": "4#E", "content": "x"}]},
               edit_refusal("tag_changed"))
        b.call("edit", {"edits": [{"to": "4#E", "from": "3#D", "content": "x"}], "hash": "ab12", "path": "/w/a.txt"},
               edit_refusal("tag_changed"))
        b.write(self.root)

        self.assertEqual(self.analyze()["retries"]["retry_unchanged"], 1)

    def test_failed_read_does_not_inform_a_retry(self) -> None:
        """A read that errored observed nothing; the legacy code reset on it anyway."""
        b = Builder()
        b.call("edit", edit_args("/w/a.txt", frm="3#DEAD"), edit_refusal("snapshot_consumed"))
        b.call("read", {"path": "/w/a.txt", "mode": "edit"}, "no such file or directory: /w/a.txt")
        b.call("edit", edit_args("/w/a.txt", frm="9#DEAD"), edit_refusal("snapshot_consumed"))
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["retries"]["retry_corrected_uninformed"], 1)
        self.assertEqual(data["retries"]["retry_informed"], 0)
        # Legacy: any read call at all cleared the flag, so it saw no blind retry.
        self.assertEqual(data["retries"]["legacy_blind_retries"], 0)

    def test_successful_editable_read_informs_a_retry(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt", frm="3#DEAD"), edit_refusal("snapshot_evicted"))
        b.call("read", {"path": "/w/a.txt", "mode": "edit"}, read_edit_ok("a.txt"))
        b.call("edit", edit_args("/w/a.txt", frm="2#DEAD"), edit_ok("a.txt"))
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["retries"]["retry_informed"], 1)
        self.assertEqual(data["retries"]["retry_corrected_uninformed"], 0)

    def test_view_read_does_not_inform_a_retry(self) -> None:
        """mode="view" carries no LINE#HASH anchors, so it grants nothing."""
        b = Builder()
        b.call("edit", edit_args("/w/a.txt", frm="3#DEAD"), edit_refusal("anchor_not_observed"))
        b.call("read", {"path": "/w/a.txt"}, "@read a.txt (10 lines, 200 bytes, showing 1-10)\n1|first\n")
        b.call("edit", edit_args("/w/a.txt", frm="5#DEAD"), edit_ok("a.txt"))
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["retries"]["retry_corrected_uninformed"], 1)
        self.assertEqual(data["retries"]["retry_informed"], 0)

    def test_editable_grep_informs_a_retry(self) -> None:
        """grep prints @file path#TAG headers and mints anchors like a read."""
        b = Builder(cwd="/w")
        b.call("edit", edit_args("/w/sub/a.txt", frm="3#DEAD"), edit_refusal("tag_changed"))
        b.call("grep", {"pattern": "match", "path": "/w"}, grep_ok("sub/a.txt"))
        b.call("edit", edit_args("/w/sub/a.txt", frm="7#DEAD"), edit_ok("sub/a.txt"))
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["retries"]["retry_informed"], 1)

    def test_editable_grep_without_a_search_path_informs_a_retry(self) -> None:
        b = Builder(cwd="/w")
        b.call("edit", edit_args("/w/sub/a.txt", frm="3#DEAD"), edit_refusal("tag_changed"))
        b.call("grep", {"pattern": "match"}, grep_ok("sub/a.txt"))
        b.call("edit", edit_args("/w/sub/a.txt", frm="7#DEAD"), edit_ok("sub/a.txt"))
        b.write(self.root)

        self.assertEqual(self.analyze()["retries"]["retry_informed"], 1)

    def test_grep_without_a_matching_header_does_not_inform(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt", frm="3#DEAD"), edit_refusal("tag_changed"))
        b.call("grep", {"pattern": "match", "path": "/w"}, grep_ok("other.txt"))
        b.call("edit", edit_args("/w/a.txt", frm="7#DEAD"), edit_ok("a.txt"))
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["retries"]["retry_corrected_uninformed"], 1)
        self.assertEqual(data["retries"]["retry_informed"], 0)

    def test_post_write_grant_informs_a_retry(self) -> None:
        """A successful write mints a grant: write -> edit needs no re-read."""
        b = Builder()
        b.call("edit", edit_args("/w/a.txt", frm="3#DEAD"), edit_refusal("no_capability"))
        b.call("write", {"path": "/w/a.txt", "content": "x"}, write_ok("a.txt"))
        b.call("edit", edit_args("/w/a.txt", frm="1#C0DE"), edit_ok("a.txt"))
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["retries"]["retry_informed"], 1)
        self.assertEqual(data["fallbacks"]["edit_fail_then_write"], 1)

    def test_write_without_returned_anchors_does_not_inform_a_retry(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt", frm="3#DEAD"), edit_refusal("no_capability"))
        b.call("write", {"path": "/w/a.txt", "content": "x"}, "wrote 42 bytes to /w/a.txt")
        b.call("edit", edit_args("/w/a.txt", frm="1#C0DE"), edit_ok("a.txt"))
        b.write(self.root)

        self.assertEqual(self.analyze()["retries"]["retry_corrected_uninformed"], 1)

    def test_retry_state_is_per_path(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), edit_refusal("stale_anchors"))
        b.call("read", {"path": "/w/b.txt", "mode": "edit"}, read_edit_ok("b.txt"))
        b.call("edit", edit_args("/w/a.txt", frm="8#DEAD"), edit_ok("a.txt"))
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["retries"]["retry_corrected_uninformed"], 1)
        self.assertEqual(data["retries"]["retry_informed"], 0)

    def test_equivalent_absolute_and_relative_paths_share_a_retry_chain(self) -> None:
        b = Builder(cwd="/w")
        b.call("edit", edit_args("/w/sub/a.txt"), edit_refusal("stale_anchors"))
        b.call("read", {"path": "sub/a.txt", "mode": "edit"}, read_edit_ok("sub/a.txt"))
        b.call("edit", edit_args("sub/a.txt", frm="7#DEAD"), edit_ok("sub/a.txt"))
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["retries"]["retry_informed"], 1)
        self.assertEqual(data["success_kinds"]["recovered"], 1)

    def test_relative_path_does_not_alias_a_different_absolute_path(self) -> None:
        b = Builder(cwd="/workspace/two")
        b.call("edit", edit_args("/workspace/one/a.txt"), edit_refusal("stale_anchors"))
        b.call("read", {"path": "a.txt", "mode": "edit"}, read_edit_ok("a.txt"))
        b.call("edit", edit_args("/workspace/two/a.txt", frm="7#DEAD"), edit_ok("a.txt"))
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["retries"]["retry_informed"], 0)
        self.assertEqual(data["retries"]["retry_corrected_uninformed"], 0)
        self.assertEqual(data["success_kinds"]["exact"], 1)

    def test_retry_state_is_per_session(self) -> None:
        first = Builder()
        first.call("edit", edit_args("/w/a.txt"), edit_refusal("stale_anchors"))
        first.write(self.root, name="2026-09-05-one.jsonl")
        second = Builder()
        second.call("edit", edit_args("/w/a.txt", frm="8#DEAD"), edit_ok("a.txt"))
        second.write(self.root, name="2026-09-05-two.jsonl")

        data = self.analyze()
        self.assertEqual(sum(data["retries"][k] for k in aee.RETRY_KINDS), 0)
        self.assertEqual(data["success_kinds"]["exact"], 1)


class SuccessKindTest(AnalyzerTestCase):
    def test_exact_rebased_and_recovered(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), edit_ok("a.txt"))
        b.call(
            "edit",
            edit_args("/w/b.txt"),
            edit_ok("b.txt", notice="rebased edits[0] from 3-4 to 5-6 (delta +2)"),
        )
        b.call("edit", edit_args("/w/c.txt"), edit_refusal("stale_anchors"))
        b.call("edit", edit_args("/w/c.txt", frm="6#DEAD"), edit_ok("c.txt"))
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["success_kinds"], {"exact": 1, "rebased": 1, "recovered": 1})
        self.assertEqual(data["edit_outcomes"], {"exact": 1, "rebased": 1, "recovered": 1, "refused": 1})

    def test_zero_delta_rebase_notice_is_still_exact(self) -> None:
        b = Builder()
        b.call(
            "edit",
            edit_args("/w/a.txt"),
            edit_ok("a.txt", notice="rebased edits[0] from 3-4 to 3-4 (delta +0)"),
        )
        b.write(self.root)

        self.assertEqual(self.analyze()["success_kinds"]["exact"], 1)

    def test_stable_success_marker_overrides_legacy_notice(self) -> None:
        b = Builder()
        b.call(
            "edit",
            edit_args("/w/a.txt"),
            edit_ok("a.txt", notice="[edit:exact]\nrebased edits[0] from 3-4 to 5-6 (delta +2)"),
        )
        b.call("edit", edit_args("/w/b.txt"), edit_ok("b.txt", notice="[edit:rebased]"))
        b.write(self.root)

        self.assertEqual(self.analyze()["success_kinds"], {"exact": 1, "rebased": 1, "recovered": 0})

    def test_marker_outside_header_metadata_is_not_a_success_marker(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), edit_ok("a.txt", notice="note\n[edit:rebased]"))
        b.write(self.root)

        self.assertEqual(self.analyze()["success_kinds"]["exact"], 1)

    def test_plan_auto_bound_uses_stable_marker_with_legacy_fallback(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), edit_ok("a.txt", notice="[plan gate] [plan:auto_bound] step repaired"))
        b.call("edit", edit_args("/w/b.txt"), edit_ok("b.txt", notice="plan step auto-bound to 'edit'"))
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["plan_auto_bound"], 2)
        self.assertEqual(data["cohorts"][0]["plan_auto_bound"], 2)


class FallbackTest(AnalyzerTestCase):
    def test_edit_fail_then_write_requires_order(self) -> None:
        """A write BEFORE the failed edit is not a fallback; the legacy set says it is."""
        b = Builder()
        b.call("write", {"path": "/w/a.txt", "content": "x"}, write_ok("a.txt"))
        b.call("edit", edit_args("/w/a.txt"), edit_refusal("tag_changed"))
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["fallbacks"]["edit_fail_then_write"], 0)
        self.assertEqual(data["edit_fail_then_write"], 1)  # legacy: unordered intersection

    def test_write_after_successful_edit(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), edit_ok("a.txt"))
        b.call("write", {"path": "/w/a.txt", "content": "x"}, write_ok("a.txt"))
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["fallbacks"]["write_after_edit_ok"], 1)
        self.assertEqual(data["fallbacks"]["edit_fail_then_write"], 0)

    def test_shell_fallback_after_a_failed_edit(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), edit_refusal("no_capability"))
        b.call("bash", {"command": "sed -i '' 's/old/new/' /w/a.txt"}, "")
        b.write(self.root)

        self.assertEqual(self.analyze()["fallbacks"]["shell_fallback"], 1)

    def test_read_only_shell_command_is_not_a_fallback(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), edit_refusal("no_capability"))
        b.call("bash", {"command": "grep -n old /w/a.txt"}, "3:old\n")
        b.write(self.root)

        self.assertEqual(self.analyze()["fallbacks"]["shell_fallback"], 0)

    def test_shell_command_without_a_pending_failure_is_not_a_fallback(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), edit_ok("a.txt"))
        b.call("bash", {"command": "sed -i '' 's/old/new/' /w/a.txt"}, "")
        b.write(self.root)

        self.assertEqual(self.analyze()["fallbacks"]["shell_fallback"], 0)


class CategoryTest(AnalyzerTestCase):
    def test_every_known_class_and_code_has_a_stable_category(self) -> None:
        names = {klass for klass, _ in aee.ERROR_CLASSES}
        names |= set(KNOWN_TYPED_CODES)
        names |= self.typed_codes_from_go()
        names |= {"canceled", "unclassified"}
        # "granted" is an outcome, not a failure; NON_ERROR_CODES records that
        # decision so a forgotten mapping cannot hide behind it.
        names -= aee.NON_ERROR_CODES
        self.assertIn("granted", aee.NON_ERROR_CODES)
        missing = sorted(n for n in names if aee.category_of(n) == "uncategorized")
        self.assertEqual(missing, [], f"codes without a stable category: {missing}")

    @staticmethod
    def typed_codes_from_go() -> set[str]:
        """Scrape the harness so a newly added refusal code fails this test."""
        codes: set[str] = set()
        hashline = REPO_ROOT / "internal" / "tools" / "writetool" / "hashline.go"
        if hashline.is_file():
            codes.update(re.findall(r'Code:\s*"([a-z_]+)"', hashline.read_text(encoding="utf-8")))
        ledger = REPO_ROOT / "internal" / "tools" / "editledger" / "ledger.go"
        if ledger.is_file():
            body = re.search(
                r"func \(o Outcome\) Code\(\) string \{(.*?)\n\}", ledger.read_text(encoding="utf-8"), re.DOTALL
            )
            if body:
                codes.update(re.findall(r'return "([a-z_]+)"', body.group(1)))
        return codes

    def test_legacy_and_typed_refusals_share_one_category(self) -> None:
        """The wire format changed; the semantic bucket must not."""
        legacy = Builder()
        legacy.call(
            "edit",
            edit_args("/w/a.txt"),
            "edit refused: the file is not authorized by a current-session editable read",
        )
        legacy.write(self.root, name="2026-09-05-legacy.jsonl")
        typed = Builder()
        typed.call("edit", edit_args("/w/b.txt"), edit_refusal("snapshot_consumed"))
        typed.write(self.root, name="2026-09-05-typed.jsonl")

        cats = self.analyze()["categories"]
        self.assertEqual(cats["totals"]["capability"], 2)
        self.assertEqual(cats["denominator"], 2)
        self.assertEqual(cats["by_cohort_tag"][aee.COHORT_TYPED]["capability"], 1)
        self.assertEqual(cats["by_cohort_tag"][aee.COHORT_LEGACY]["capability"], 1)
        # Legacy class names still split the same two failures apart.
        self.assertEqual(
            self.analyze()["classes"], {"no_capability": 1, "snapshot_consumed": 1}
        )

    def test_unmapped_code_is_reported_not_dropped(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), edit_refusal("brand_new_code"))
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["uncategorized_codes"], {"brand_new_code": 1})
        self.assertEqual(data["categories"]["totals"]["uncategorized"], 1)
        buffer = io.StringIO()
        with redirect_stdout(buffer):
            aee.print_report(data)
        first_line = buffer.getvalue().splitlines()[0]
        self.assertIn("no stable category", first_line)
        self.assertIn("brand_new_code", buffer.getvalue())

    def test_non_edit_tools_stay_out_of_the_legacy_sections(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), edit_ok("a.txt"))
        b.call("grep", {"pattern": "x", "path": "/w"}, "path not found: /w. Check the path and try again")
        b.call("bash", {"command": "false"}, "\n(exit error: exit status 1)")
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["classes"], {})
        self.assertEqual(data["per_tool_errors"], {})
        self.assertEqual(data["categories"]["totals"], {})
        self.assertEqual(data["tools"]["grep"]["error"], 1)
        self.assertEqual(data["tools"]["bash"]["error"], 1)

    def test_read_errors_remain_legacy_classes_but_not_edit_categories(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), edit_ok("a.txt"))
        b.call("read", {"path": "/w/a.txt", "mode": "edit"}, "the plan is not approved")
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["classes"], {"plan_gate": 1})
        self.assertEqual(data["per_tool_errors"], {"read": {"plan_gate": 1}})
        self.assertEqual(data["categories"]["denominator"], 1)
        self.assertEqual(data["categories"]["totals"], {})
        self.assertEqual(data["cohorts"][0]["categories"], {})

    def test_canceled_calls_are_their_own_category(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), aee.CANCELED_TEXT)
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["classes"], {"canceled": 1})
        self.assertEqual(data["categories"]["totals"]["canceled"], 1)


class AttributionTest(AnalyzerTestCase):
    def test_entry_model_wins_over_header(self) -> None:
        b = Builder(header_model="header-model", default_model="entry-model")
        b.call("edit", edit_args("/w/a.txt"), edit_ok("a.txt"))
        b.write(self.root)

        cohorts = self.analyze()["cohorts"]
        self.assertEqual(len(cohorts), 1)
        self.assertEqual(cohorts[0]["model"], "entry-model")

    def test_header_model_is_the_fallback(self) -> None:
        b = Builder(header_model="header-model", default_model="")
        b.call("edit", edit_args("/w/a.txt"), edit_ok("a.txt"))
        b.write(self.root)

        self.assertEqual(self.analyze()["cohorts"][0]["model"], "header-model")

    def test_missing_model_is_unknown_not_dropped(self) -> None:
        b = Builder(default_model="")
        b.call("edit", edit_args("/w/a.txt"), edit_ok("a.txt"))
        b.write(self.root)

        cohorts = self.analyze()["cohorts"]
        self.assertEqual(cohorts[0]["model"], "unknown")
        self.assertEqual(cohorts[0]["edit_attempts"], 1)

    def test_usage_and_latency_are_collected(self) -> None:
        b = Builder(effort="high")
        b.call(
            "edit",
            edit_args("/w/a.txt"),
            edit_refusal("stale_anchors"),
            usage={"prompt_tokens": 100, "completion_tokens": 10, "total_tokens": 110},
            latency_ms=1000,
        )
        b.call(
            "edit",
            edit_args("/w/a.txt", frm="9#DEAD"),
            edit_ok("a.txt"),
            usage={"prompt_tokens": 200, "completion_tokens": 20, "total_tokens": 220},
            latency_ms=3000,
        )
        b.write(self.root)

        row = self.analyze()["cohorts"][0]
        self.assertEqual(row["effort"], "high")
        self.assertEqual(row["input_tokens"], 300)
        self.assertEqual(row["output_tokens"], 30)
        # Timed from the assistant entry to its tool result: tool execution.
        self.assertEqual(row["tool_latency_ms_median"], 2000.0)
        self.assertEqual(row["tool_latency_ms_p90"], 2800.0)

    def test_response_usage_is_not_duplicated_for_parallel_tool_calls(self) -> None:
        b = Builder()
        b.batch(
            [
                ("edit", edit_args("/w/a.txt"), edit_ok("a.txt")),
                ("edit", edit_args("/w/b.txt"), edit_ok("b.txt")),
            ],
            usage={"prompt_tokens": 100, "completion_tokens": 10, "total_tokens": 110},
        )
        b.batch([], usage={"prompt_tokens": 50, "completion_tokens": 5, "total_tokens": 55})
        b.write(self.root)

        row = self.analyze()["cohorts"][0]
        self.assertEqual((row["input_tokens"], row["output_tokens"]), (150, 15))
        self.assertEqual(row["reported_total_tokens"], 165)
        self.assertEqual(row["usage_runs"], 1)


class CohortTest(AnalyzerTestCase):
    def _eval_tree(self, *, correct: bool, model: str, harness: str, scenario: str, name: str) -> None:
        run_dir = self.root / harness / model / scenario / name
        run_dir.mkdir(parents=True, exist_ok=True)
        (run_dir / "manifest.json").write_text(
            json.dumps(
                {
                    "model": "manifest-model",
                    "model_version": "2026-09-05",
                    "effort": "high",
                    "harness": harness,
                    "harness_revision": "d806cc9",
                    "scenario": scenario,
                    "run": 1,
                    "task_success": correct,
                    "exit_code": 0,
                    "elapsed_ms": 1234,
                    "usage": {"prompt_tokens": 400, "completion_tokens": 40, "total_tokens": 440},
                    "cost_usd": 0.01234,
                }
            ),
            encoding="utf-8",
        )
        b = Builder(default_model=model)
        b.call("edit", edit_args("/w/a.txt"), edit_ok("a.txt"))
        b.write(run_dir, name="2026-09-05-run.jsonl")

    def test_manifest_supplies_harness_scenario_and_correctness(self) -> None:
        self._eval_tree(correct=True, model="strong", harness="HEAD", scenario="exact-single-edit", name="run-1")
        self._eval_tree(correct=False, model="weak", harness="HEAD", scenario="exact-single-edit", name="run-2")

        rows = {str(r["cohort"]): r for r in self.analyze()["cohorts"]}
        self.assertIn("strong@2026-09-05|high|HEAD@d806cc9|exact-single-edit", rows)
        self.assertIn("weak@2026-09-05|high|HEAD@d806cc9|exact-single-edit", rows)
        strong = rows["strong@2026-09-05|high|HEAD@d806cc9|exact-single-edit"]
        # The transcript's own model wins; the manifest only fills the blanks.
        self.assertEqual(strong["model"], "strong")
        self.assertEqual((strong["correct"], strong["graded"]), (1, 1))
        self.assertEqual((strong["run_elapsed_ms"], strong["reported_cost_usd"]), (1234.0, 0.01234))
        self.assertEqual((strong["reported_input_tokens"], strong["reported_output_tokens"]), (400, 40))
        weak = rows["weak@2026-09-05|high|HEAD@d806cc9|exact-single-edit"]
        self.assertEqual((weak["correct"], weak["graded"]), (0, 1))

    def test_no_manifest_means_unlabeled_and_organic(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), edit_ok("a.txt"))
        b.write(self.root)

        row = self.analyze()["cohorts"][0]
        self.assertEqual((row["harness"], row["scenario"]), ("unlabeled", "organic"))
        self.assertEqual(row["graded"], 0)

    def test_manifest_only_run_keeps_a_zero_tool_call_cohort(self) -> None:
        run_dir = self.root / "HEAD" / "weak" / "no-tool-needed" / "run-1"
        run_dir.mkdir(parents=True)
        (run_dir / "manifest.json").write_text(
            json.dumps(
                {
                    "model": "weak",
                    "model_version": "2026-09-05",
                    "effort": "low",
                    "harness": "HEAD",
                    "harness_revision": "d806cc9",
                    "scenario": "no-tool-needed",
                    "task_success": False,
                    "elapsed_ms": 55,
                }
            ),
            encoding="utf-8",
        )
        Builder().write(run_dir, name="2026-09-05-run.jsonl")

        rows = self.analyze()["cohorts"]
        self.assertEqual(len(rows), 1)
        row = rows[0]
        self.assertEqual(row["cohort"], "weak@2026-09-05|low|HEAD@d806cc9|no-tool-needed")
        self.assertEqual((row["sessions"], row["edit_attempts"]), (1, 0))
        self.assertEqual((row["correct"], row["graded"], row["run_elapsed_ms"]), (0, 1, 55.0))

    def test_cohort_rows_carry_denominators_and_kinds(self) -> None:
        b = Builder(default_model="weak")
        b.call("edit", edit_args("/w/a.txt"), edit_refusal("stale_anchors"))
        b.call("edit", edit_args("/w/a.txt"), edit_refusal("stale_anchors"))
        b.call("read", {"path": "/w/a.txt", "mode": "edit"}, read_edit_ok("a.txt"))
        b.call("edit", edit_args("/w/a.txt", frm="2#DEAD"), edit_ok("a.txt"))
        b.write(self.root)

        row = self.analyze()["cohorts"][0]
        self.assertEqual(row["edit_attempts"], 3)
        self.assertEqual(row["tool_calls"], {"edit": 3, "read": 1})
        self.assertEqual(row["edit_errors"], 2)
        self.assertEqual(row["edit_error_rate_pct"], 66.7)
        self.assertEqual(row["retries"]["retry_unchanged"], 1)
        self.assertEqual(row["retries"]["retry_informed"], 1)
        self.assertEqual(row["success_kinds"]["recovered"], 1)
        self.assertEqual(row["categories"], {"stale": 2})


class ParsingTest(AnalyzerTestCase):
    def test_two_tool_calls_in_one_message(self) -> None:
        b = Builder()
        b.batch(
            [
                ("edit", edit_args("/w/a.txt"), edit_ok("a.txt")),
                ("edit", edit_args("/w/b.txt"), edit_refusal("stale_anchors")),
            ]
        )
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["tools"]["edit"]["calls"], 2)
        self.assertEqual(data["tools"]["edit"]["success"], 1)
        self.assertEqual(data["tools"]["edit"]["error"], 1)

    def test_unmatched_call_is_no_result(self) -> None:
        b = Builder()
        b.lines.append(
            json.dumps(
                {
                    "type": "EntryMessage",
                    "timestamp": BASE_TIME.isoformat(),
                    "message": {
                        "role": "assistant",
                        "tool_calls": [
                            {
                                "id": "orphan",
                                "type": "function",
                                "function": {"name": "edit", "arguments": json.dumps(edit_args("/w/a.txt"))},
                            }
                        ],
                    },
                }
            )
        )
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["tools"]["edit"]["no_result"], 1)
        self.assertEqual(data["classes"], {})

    def test_bad_lines_are_counted_not_fatal(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), edit_ok("a.txt"))
        b.lines.append("{not json")
        b.write(self.root)

        data = self.analyze()
        self.assertEqual(data["corpus"]["bad_lines"], 1)
        self.assertEqual(data["tools"]["edit"]["calls"], 1)

    def test_job_transcripts_are_included(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), edit_ok("a.txt"))
        job = self.root / "jobs" / "job1"
        b.write(job, name="2026-09-05-job.jsonl")

        data = self.analyze()
        self.assertEqual(data["corpus"]["jobs"], 1)
        self.assertEqual(data["tools"]["edit"]["calls"], 1)

    def test_reports_are_json_serializable(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), edit_refusal("stale_anchors"))
        b.write(self.root)

        payload = json.loads(json.dumps(self.analyze()))
        self.assertEqual(payload["categories"]["totals"]["stale"], 1)

    def test_tool_result_text_is_not_emitted(self) -> None:
        secret = "not-for-report-7c8050"
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), f"unrecognized tool output: {secret}")
        b.write(self.root)

        data = self.analyze()
        self.assertNotIn(secret, json.dumps(data))
        buffer = io.StringIO()
        with redirect_stdout(buffer):
            aee.print_report(data)
        self.assertNotIn(secret, buffer.getvalue())


class BaselineTest(AnalyzerTestCase):
    def test_baseline_diff_prints_rates_with_denominators(self) -> None:
        before = Builder()
        before.call("edit", edit_args("/w/a.txt"), edit_refusal("no_capability"))
        before.call("edit", edit_args("/w/a.txt", frm="9#DEAD"), edit_ok("a.txt"))
        before.write(self.root)
        baseline = self.analyze()

        with tempfile.TemporaryDirectory() as second:
            after_root = Path(second)
            after = Builder()
            after.call("edit", edit_args("/w/a.txt"), edit_ok("a.txt"))
            after.write(after_root)
            current = aee.analyze(after_root, 2)

        buffer = io.StringIO()
        with redirect_stdout(buffer):
            aee.print_baseline_diff(current, baseline)
        text = buffer.getvalue()
        self.assertIn("edit attempts: 2 -> 1", text)
        self.assertIn("capability     1/2 (50.0%) -> 0/1 (0.0%)  delta -50.0pp", text)
        self.assertIn("retry_corrected_uninformed", text)

    def test_cli_round_trip_with_baseline(self) -> None:
        b = Builder()
        b.call("edit", edit_args("/w/a.txt"), edit_refusal("stale_anchors"))
        b.write(self.root)
        saved = self.root / "baseline.json"

        buffer = io.StringIO()
        with redirect_stdout(buffer):
            code = aee.main(["--root", str(self.root), "--json"])
        self.assertEqual(code, 0)
        saved.write_text(buffer.getvalue(), encoding="utf-8")

        buffer = io.StringIO()
        with redirect_stdout(buffer):
            code = aee.main(["--root", str(self.root), "--baseline", str(saved)])
        self.assertEqual(code, 0)
        self.assertIn("baseline comparison", buffer.getvalue())

    def test_missing_root_exits_with_two(self) -> None:
        errors = io.StringIO()
        with redirect_stderr(errors):
            code = aee.main(["--root", str(self.root / "nope")])
        self.assertEqual(code, 2)
        self.assertIn("not a directory", errors.getvalue())


if __name__ == "__main__":
    unittest.main()
