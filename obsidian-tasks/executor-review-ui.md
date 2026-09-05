---
id: executor-review-ui
title: Inspect executor review states and target session actions
status: blocked
priority: medium
model_level: medium
task_type: feature
parent_id: plan-driven-interactive-executors
tags:
    - executors
    - tui
acceptance_criteria:
    - Users can inspect assignment/local-plan review state, pending result acceptance and the correct transcript.
    - Explicit human resume targets the human-stopped executor shown in the view.
    - Job done is distinct from accepted; busy, stopped and conflicted states reflect runtime truth.
    - Actions retain correct origin without focus theft or changing a sibling model.
    - Widgets do not implement their own authority checks; approved state labels/help copy and fixtures are supplied for slice 10.
verification_plan:
    - Use public input-to-controller-to-render tests with two children, background outcome, review/rework and explicit human resume.
    - Check keyboard/mouse behavior via existing navigation.
    - Verify origin targeting, no focus theft, model isolation and distinct completion/acceptance rendering.
created_at: "2026-09-05T23:21:30.483849Z"
updated_at: "2026-09-05T23:21:30.483849Z"
---

## Body

**Approved slice:** 09 of [breakdown](../specs/plan-driven-executors-tickets.md). Read [contract](../specs/plan-driven-executors.md), D5–D8.

**Blocked by:** executor-human-authority.

**Capability rationale:** medium — authenticated transitions/projection are fixed; wire existing selection and actions through them.

**Inputs:** slice 05 typed projection and existing retained View/selector controls.

**Scope/output:** inspect executor assignment and approach/result review state, reach its transcript and explicitly resume a human-stopped executor. Reuse existing controls; broader sidebar/notifications remain owned by multisession-background-attention. Freeze exact explanations and fixtures for the bounded low-capability follow-up.

**Escalation:** missing runtime transitions return to the human-authority owner, not widget code. Full sidebar delivery is not a prerequisite.

## Acceptance Criteria

- Users can inspect assignment/local-plan review state, pending result acceptance and the correct transcript.
- Explicit human resume targets the human-stopped executor shown in the view.
- Job done is distinct from accepted; busy, stopped and conflicted states reflect runtime truth.
- Actions retain correct origin without focus theft or changing a sibling model.
- Widgets do not implement their own authority checks; approved state labels/help copy and fixtures are supplied for slice 10.

## Verification Plan

1. Use public input-to-controller-to-render tests with two children, background outcome, review/rework and explicit human resume.
2. Check keyboard/mouse behavior via existing navigation.
3. Verify origin targeting, no focus theft, model isolation and distinct completion/acceptance rendering.
