---
id: executor-result-acceptance
title: Accept executor results against current artifact-bound evidence
status: blocked
priority: high
model_level: high
task_type: feature
parent_id: plan-driven-interactive-executors
tags:
    - executors
    - plan
    - acceptance
acceptance_criteria:
    - Job completion and child local-plan completion never close the parent step.
    - Parent acceptance checks the current contract, approved plan and artifact-bound evidence.
    - Changed dirty, untracked or deleted artifacts invalidate stale evidence.
    - Duplicate or stale decisions cannot complete the parent step twice; unresolved findings remain visible.
verification_plan:
    - Exercise accept and return through the public runtime and plan.
    - Test committed and dirty manifests including untracked and deleted artifacts.
    - Mutate an artifact between verification and acceptance; require stale-evidence refusal.
    - Inject duplicate outcomes, stale contracts and receipt recovery failures; verify atomic, idempotent parent-step state.
created_at: "2026-09-05T23:20:03.083779Z"
updated_at: "2026-09-05T23:20:03.083779Z"
---

## Body

**Approved slice:** 03 of [breakdown](../specs/plan-driven-executors-tickets.md). Read [contract](../specs/plan-driven-executors.md), D6–D7 and D9.

**Blocked by:** executor-local-plan-review. Resume only after its contract and fixtures are accepted.

**Capability rationale:** high — durable acceptance and artifact mutation need a precise transaction and evidence identity, not a trusted child summary.

**Inputs:** slice 02 identities/revisions, existing plan evidence and outcome receipt paths, owned-worktree conventions.

**Scope/output:** completed execution presents a candidate; the parent accepts or returns findings against the current contract and checked artifacts. Finalize manifest, durable result/review records and atomic acceptance checks. Include an explicit same-session next-attempt contract for slice 04; no separate success scheduler.

**Escalation:** inability to establish safe atomic acceptance or artifact identity requires design review, not a verified flag or broad whole-workspace hash. Supply public fixtures for downstream negative cases.

## Acceptance Criteria

- Job completion and child local-plan completion never close the parent step.
- Parent acceptance checks the current contract, approved plan and artifact-bound evidence.
- Changed dirty, untracked or deleted artifacts invalidate stale evidence.
- Duplicate or stale decisions cannot complete the parent step twice; unresolved findings remain visible.

## Verification Plan

1. Exercise accept and return through the public runtime and plan.
2. Test committed and dirty manifests including untracked and deleted artifacts.
3. Mutate an artifact between verification and acceptance; require stale-evidence refusal.
4. Inject duplicate outcomes, stale contracts and receipt recovery failures; verify atomic, idempotent parent-step state.
