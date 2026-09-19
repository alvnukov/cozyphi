---
id: web-research-44-lifecycle-regressions
title: 44 — Add deterministic lifecycle, replay and revocation regressions
status: blocked
priority: high
model_level: low
task_type: test
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Fixed public lifecycle regressions cover stop/restore/fork/compaction, late/duplicate events and cross-process revocation.
    - Recheck/unblock cannot be replayed from stale authority, and delivered derivatives cannot regain authority after source/resource blocks.
    - Synchronization is deterministic; evidence checks actual delivery/request/action sinks rather than private state alone.
verification_plan:
    - Run only the fixed lifecycle/revocation cases with channel/process barriers and fake clock.
    - Inspect effective next-request payloads, delivery counts and action sinks after each transition.
    - Record scoped race results, positive controls and restored fault-injection evidence; no sleep/retry-to-pass workaround.
created_at: "2026-09-19T19:22:50.602729Z"
updated_at: "2026-09-19T19:22:50.602729Z"
---

## Body

**What to build:** A bounded regression extension for implemented lifecycle and revocation contracts.

**Blocked by:** [22](web-research-22-explicit-resume.md), [24](web-research-24-durable-lineage.md), [28](web-research-28-inflight-revocation.md), [29](web-research-29-delivered-revocation.md), [31](web-research-31-user-resolution.md), [40](web-research-40-subresource-revocation.md).

**Contract:** [Spec](../specs/protected-web-research.md), Testing Decisions, T19–T22/T26/T27/T32. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Add named scenarios to existing public lifecycle fixtures using controlled completions, process coordination and fake clock. Cover late completion after cancel/restart, duplicate delivery, new real user input, fork/compaction/child derivatives, block while queued and stale unblock confirmation. Keep each fixture small and deterministic; use positive independent-context controls. This is extra integration coverage, not deferred mandatory tests for earlier tasks.

**Do not change:** No redesign of session graph, sleep-based ordering, invented successful recovery or broad feature implementation.

**Proof required:** Event/operation sequence, expected/actual consumer and action outcomes, exact revision/platform and scoped race-test results. Representative missing-check fault injection must make the relevant case fail and then be removed.

**Stop condition:** Missing lifecycle API or production defect is a blocking owner issue, not permission to assert a private flag instead.

## Acceptance Criteria

- Fixed public lifecycle regressions cover stop/restore/fork/compaction, late/duplicate events and cross-process revocation.
- Recheck/unblock cannot be replayed from stale authority, and delivered derivatives cannot regain authority after source/resource blocks.
- Synchronization is deterministic; evidence checks actual delivery/request/action sinks rather than private state alone.

## Verification Plan

1. Run only the fixed lifecycle/revocation cases with channel/process barriers and fake clock.
2. Inspect effective next-request payloads, delivery counts and action sinks after each transition.
3. Record scoped race results, positive controls and restored fault-injection evidence; no sleep/retry-to-pass workaround.
