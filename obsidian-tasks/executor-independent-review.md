---
id: executor-independent-review
title: Request independent artifact review without parent conclusions
status: blocked
priority: medium
model_level: medium
task_type: feature
parent_id: plan-driven-interactive-executors
tags:
    - executors
    - review
    - context
acceptance_criteria:
    - Independent review excludes parent conclusions by default while preserving original user constraints.
    - The selected and excluded context sources are visible in a manifest.
    - Findings reference the reviewed artifacts; unread material is not reported as reviewed.
    - The reviewer uses local-plan review, stays read-only, cannot self-accept the parent step and is explicitly distinct from full executor context.
verification_plan:
    - Use a fixture with a planted incorrect parent conclusion and contradicting artifact; assert actual provider input and result identity.
    - Test permission-gate and artifact-access denial cases.
    - Check source manifest completeness and that reviewer completion does not accept the parent step.
created_at: "2026-09-05T23:20:46.985047Z"
updated_at: "2026-09-05T23:20:46.985047Z"
---

## Body

**Approved slice:** 06 of [breakdown](../specs/plan-driven-executors-tickets.md). Read [contract](../specs/plan-driven-executors.md), D4, D7 and D10.

**Blocked by:** executor-result-acceptance.

**Capability rationale:** medium — snapshot/authority mechanics exist; add one explicit source-selection mode with a testable manifest, not a new lifecycle.

**Inputs:** slice 01 snapshot interface and slice 03 artifact/evidence envelope.

**Scope/output:** launch a read-only review with requirements, artifacts and evidence, expose selected/excluded sources and return artifact-bound findings. Reuse local-plan approval. No nested reviewer delegation.

**Escalation:** if provenance cannot distinguish requirements from conclusions, request explicit source selection/review; do not claim automatic independence.

## Acceptance Criteria

- Independent review excludes parent conclusions by default while preserving original user constraints.
- The selected and excluded context sources are visible in a manifest.
- Findings reference the reviewed artifacts; unread material is not reported as reviewed.
- The reviewer uses local-plan review, stays read-only, cannot self-accept the parent step and is explicitly distinct from full executor context.

## Verification Plan

1. Use a fixture with a planted incorrect parent conclusion and contradicting artifact; assert actual provider input and result identity.
2. Test permission-gate and artifact-access denial cases.
3. Check source manifest completeness and that reviewer completion does not accept the parent step.
