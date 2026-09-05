---
id: executor-linked-rework
title: Route clarification and linked rework to the retained executor
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: plan-driven-interactive-executors
tags:
    - executors
    - rework
acceptance_criteria:
    - Rework and clarification retain child history and use a new immutable JobID linked to its predecessor.
    - Repeated parent requests never duplicate runs; ordinary follow-up queues rather than interrupts.
    - Each attempt carries explicit timeout and current contract/context delta.
    - Material rework requires renewed local-plan approval; late old outcomes cannot satisfy the current contract.
verification_plan:
    - Run two rework cycles and a clarification using public operations; check retained session and linked JobIDs.
    - Test duplicate delivery, busy queue, full admission, missing child and timeout cases with controlled barriers.
    - Verify the parent receives the current final result once and material replanning returns to approval.
created_at: "2026-09-05T23:20:17.626663Z"
updated_at: "2026-09-05T23:20:17.626663Z"
---

## Body

**Approved slice:** 04 of [breakdown](../specs/plan-driven-executors-tickets.md). Read [contract](../specs/plan-driven-executors.md), D4–D6.

**Blocked by:** executor-result-acceptance.

**Capability rationale:** medium — owner, revision and interaction decisions are fixed by slices 02–03; implement a bounded path through the existing Controller queue and Manager admission.

**Inputs:** approved routed schema, outcome kinds, rework record and tests from 02–03; existing human follow-up and wait/push dedup behavior.

**Scope/output:** the parent returns a concrete finding or answers a clarification; the same retained session starts one immutable linked attempt with explicit timeout and current contract/context delta. Use the established message and plan semantics.

**Escalation:** missing transition/schema requirements return to the foundation owner; do not invoke the Engine directly or silently replace the runtime.

## Acceptance Criteria

- Rework and clarification retain child history and use a new immutable JobID linked to its predecessor.
- Repeated parent requests never duplicate runs; ordinary follow-up queues rather than interrupts.
- Each attempt carries explicit timeout and current contract/context delta.
- Material rework requires renewed local-plan approval; late old outcomes cannot satisfy the current contract.

## Verification Plan

1. Run two rework cycles and a clarification using public operations; check retained session and linked JobIDs.
2. Test duplicate delivery, busy queue, full admission, missing child and timeout cases with controlled barriers.
3. Verify the parent receives the current final result once and material replanning returns to approval.
