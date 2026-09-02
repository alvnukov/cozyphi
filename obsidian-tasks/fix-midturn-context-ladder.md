---
id: fix-midturn-context-ladder
title: Stop runaway tool loops mid-turn with the compaction ladder and a hard window guard
status: done
priority: high
task_type: bug
tags:
    - reliability
    - compaction
    - agent
acceptance_criteria:
    - 'A tool-looping turn that crosses compaction.reminder_tokens escalates per tool round: strike 4 interrupts the runaway and runs one context-only offer round; a landed compaction rearms the ladder and the turn continues.'
    - Without a compaction in the offer round, Loop ends with ErrCompactionRequired and refuses further runs until /compact (existing compactStopped invariant).
    - 'No inference is sent when the estimated context exceeds the model window: Loop returns ErrCompactionRequired before the request.'
    - Regression tests cover both paths (mid-turn ladder escalation with offer round; over-window refusal) and make fmt-check lint test stay green.
verification_plan:
    - go test ./internal/agent/ with the new mid-turn ladder and window-guard regression tests (red before the fix, green after)
    - make fmt-check lint test in the task worktree
created_at: "2026-08-30T18:07:13.128768Z"
updated_at: "2026-08-30T18:08:30.878627Z"
---

## Body

**Symptom.** Session 55cf07d2d019304089c1e48cf1f43c4f (2026-08-30): the model entered an endless tool loop inside a single turn — 101 tool rounds in ~1h, the last assistant message carried 1025 tool calls (1024 read of the same 4 files + 1 mcp_call), estimated context reached ~211k tokens against a 200k window, and the next inference killed the provider API. The compaction escalation ladder never fired.

**Root cause.** noteCompactPressure is only called at turn end (the no-tool-calls branch of Engine.Loop), so a turn that never ends escalates nothing — the 80k reminder threshold was crossed unnoticed. There is also no hard guard before streamTurn, so an over-window request is sent regardless. maxRounds=128 does not help when a single round explodes.

**Fix.** Count ladder strikes at every tool-round boundary in Engine.Loop: 1–2 soft reminders, 3 hard (every tool blocked except context), 4 — interrupt the runaway and run one final offer round driven by a synthetic reminder-format directive (summary + compact, context-only); a landed compaction rearms the ladder and the turn continues, otherwise stop with ErrCompactionRequired and wait for the user (/compact). Plus a pre-flight guard: never send an inference whose estimated context exceeds the model window.

## Acceptance Criteria

- A tool-looping turn that crosses compaction.reminder_tokens escalates per tool round: strike 4 interrupts the runaway and runs one context-only offer round; a landed compaction rearms the ladder and the turn continues.
- Without a compaction in the offer round, Loop ends with ErrCompactionRequired and refuses further runs until /compact (existing compactStopped invariant).
- No inference is sent when the estimated context exceeds the model window: Loop returns ErrCompactionRequired before the request.
- Regression tests cover both paths (mid-turn ladder escalation with offer round; over-window refusal) and make fmt-check lint test stay green.

## Verification Plan

1. go test ./internal/agent/ with the new mid-turn ladder and window-guard regression tests (red before the fix, green after)
2. make fmt-check lint test in the task worktree
