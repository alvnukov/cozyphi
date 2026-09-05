---
id: executor-human-authority
title: Preserve human stop and reconcile executor intervention
status: blocked
priority: high
model_level: high
task_type: feature
parent_id: plan-driven-interactive-executors
tags:
    - executors
    - permissions
    - lifecycle
acceptance_criteria:
    - Explicit human stop latches parent-resume refusal until explicit human resumption.
    - Viewing is not intervention; parent messages cannot answer a human Ask.
    - Queued stale parent direction cannot overwrite newer human intent; unresolved intervention blocks result acceptance.
    - Concrete bounded intervention events reach the parent with origin/order information.
    - Stop completion and capacity release wait for actual execution exit.
verification_plan:
    - Test human stop followed by parent send, explicit human resume, and parent-stop versus human-stop behavior.
    - Use controlled barriers for simultaneous input, approval and result delivery, including queued follow-up versus human stop.
    - Verify cross-session Ask/event routing and capacity held until actual exit.
created_at: "2026-09-05T23:20:32.077115Z"
updated_at: "2026-09-05T23:20:32.077115Z"
---

## Body

**Approved slice:** 05 of [breakdown](../specs/plan-driven-executors-tickets.md). Read [contract](../specs/plan-driven-executors.md), D5–D9.

**Blocked by:** executor-linked-rework.

**Capability rationale:** high — human/parent races and resume authority cross execution, queues and acceptance; incorrect behavior can undo explicit human intent.

**Inputs:** existing origin/Ask/session fences and slice 04 routed interactions.

**Scope/output:** route human clarification, scope conflict and explicit stop as concrete bounded events. Enforce human-stop protection until explicit human resume, reconcile conflicts, and establish the typed projection needed by later UI work. Keep human and parent cancellation distinct.

**Escalation:** uncertainty about authorization fails closed with an actionable report. Never infer authority from transcript text.

## Acceptance Criteria

- Explicit human stop latches parent-resume refusal until explicit human resumption.
- Viewing is not intervention; parent messages cannot answer a human Ask.
- Queued stale parent direction cannot overwrite newer human intent; unresolved intervention blocks result acceptance.
- Concrete bounded intervention events reach the parent with origin/order information.
- Stop completion and capacity release wait for actual execution exit.

## Verification Plan

1. Test human stop followed by parent send, explicit human resume, and parent-stop versus human-stop behavior.
2. Use controlled barriers for simultaneous input, approval and result delivery, including queued follow-up versus human stop.
3. Verify cross-session Ask/event routing and capacity held until actual exit.
