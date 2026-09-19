---
id: web-research-31-user-resolution
title: 31 — Require successful recheck and a separate human unblock decision
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Only the user initiates isolated recheck of an identified snapshot and separately chooses unblock.
    - Only a current successful scope-matching report can enable the unblock decision; failed/unknown/incomplete/stale reports cannot.
    - History remains intact; successful recheck alone does not unblock, replay old answers or remove future checks.
verification_plan:
    - Drive the full recheck/unblock transition table through public human actions; try equivalent model/tool requests and deny them.
    - Race a report with policy/source/block changes and assert stale confirmation fails.
    - Inspect preserved incident history and future release checks; run scoped state/UI integration tests.
created_at: "2026-09-19T19:18:45.040716Z"
updated_at: "2026-09-19T19:18:45.040716Z"
---

## Body

**What to build:** Human-controlled resolution of a suspected false block without a raw-content bypass.

**Blocked by:** [04](web-research-04-consented-preflight.md), [27](web-research-27-host-blocks.md), [30](web-research-30-incident-viewer.md).

**Contract:** [Spec](../specs/protected-web-research.md), D9/D10, Q8/Q36/Q43, T21/T22. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Add separate host events for start-recheck and confirm-unblock. Bind the recheck to immutable incident scope, active policy/model and explicit user authority. Use the quarantine pipeline with no real executors and retain the original incident plus new report. Revalidate report currency/scope at the actual unblock transition through shared coordination. Expired/missing snapshots require an explicit permitted resolution, not a fabricated successful report. Later source use still passes current checks.

**Do not change:** No model-initiated retry-to-pass, auto-unblock, implicit clear on config change, raw delivery or restoration of old answers.

**Proof required:** A transition table exercised through UI/host commands covering failed, unknown, stale, mismatched, success-unconfirmed and success-confirmed outcomes; only the last clears the applicable automatic block.

**Stop condition:** If current incident state changes during user review, require fresh matching evidence.

## Acceptance Criteria

- Only the user initiates isolated recheck of an identified snapshot and separately chooses unblock.
- Only a current successful scope-matching report can enable the unblock decision; failed/unknown/incomplete/stale reports cannot.
- History remains intact; successful recheck alone does not unblock, replay old answers or remove future checks.

## Verification Plan

1. Drive the full recheck/unblock transition table through public human actions; try equivalent model/tool requests and deny them.
2. Race a report with policy/source/block changes and assert stale confirmation fails.
3. Inspect preserved incident history and future release checks; run scoped state/UI integration tests.
