---
id: executor-status-explanations
title: Explain executor states using the approved UI mapping
status: blocked
priority: medium
model_level: low
task_type: feature
parent_id: plan-driven-interactive-executors
tags:
    - executors
    - tui
acceptance_criteria:
    - Each approved executor projection renders its specified concise explanation.
    - Job completion never says accepted; unknown state uses the approved neutral fallback.
    - Narrow views remain readable.
    - No new state, event, action or lifecycle semantics are introduced.
verification_plan:
    - Extend existing renderer table tests using the approved exact strings.
    - Check narrow/wide fixtures and unknown-state fallback.
    - Manually inspect one selected executor view.
created_at: "2026-09-05T23:21:42.542216Z"
updated_at: "2026-09-05T23:21:42.542216Z"
---

## Body

**Approved slice:** 10 of [breakdown](../specs/plan-driven-executors-tickets.md). Read [contract](../specs/plan-driven-executors.md), D8 state/action table.

**Blocked by:** executor-review-ui.

**Capability rationale:** low — implement a single approved finite projection-to-text mapping; no lifecycle decisions.

**Inputs:** slice 09 rendered view, existing renderer fixtures, and exact labels/help copy frozen during its parent acceptance.

**Scope/output:** wire the approved explanations for waiting for parent plan review, executing, result awaiting review, changes requested, human-stopped and accepted into the selected executor view and help affordance.

**Escalation:** missing state/copy or a required layout redesign returns to the UI owner; do not guess transitions or rewrite the selector.

## Acceptance Criteria

- Each approved executor projection renders its specified concise explanation.
- Job completion never says accepted; unknown state uses the approved neutral fallback.
- Narrow views remain readable.
- No new state, event, action or lifecycle semantics are introduced.

## Verification Plan

1. Extend existing renderer table tests using the approved exact strings.
2. Check narrow/wide fixtures and unknown-state fallback.
3. Manually inspect one selected executor view.
