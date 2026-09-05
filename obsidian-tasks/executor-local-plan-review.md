---
id: executor-local-plan-review
title: Require parent approval of child local execution plans
status: blocked
priority: high
model_level: high
task_type: feature
parent_id: plan-driven-interactive-executors
tags:
    - executors
    - plan
    - approval
acceptance_criteria:
    - Before parent approval the child can only use the defined preparation allowlist; edits are denied.
    - Approved child execution still passes the ordinary permission gate.
    - Wrong parent, stale revision and fake textual approval are rejected; returned plans retain the same child session.
    - Waiting for review holds no inference slot; progress-only updates preserve approval and material changes revoke it.
    - Ordinary user-owned plans retain their existing semantics.
verification_plan:
    - Exercise proposal, return, revision and exact-revision approval through the public runtime.
    - Verify permission denial after plan approval plus wrong-owner, stale-revision and textual-approval cases.
    - Use deterministic cancellation/admission barriers to verify waiting releases inference capacity.
    - Verify progress versus material mutation approval semantics and ordinary user-owned plan compatibility.
created_at: "2026-09-05T23:19:48.317447Z"
updated_at: "2026-09-05T23:19:48.317447Z"
---

## Body

**Approved slice:** 02 of [breakdown](../specs/plan-driven-executors-tickets.md). Read [contract](../specs/plan-driven-executors.md), D1–D6.

**Blocked by:** executor-context-snapshot, plan-v2-strong-model-review, plan-v2-contract-migration-integration. These existing safety gates validate the transitions extended here. Resume only after all blockers are done.

**Capability rationale:** high — approver identity, preparation policy and version fencing are authority semantics; settle them before bounded downstream implementation.

**Inputs:** reviewed Plan v2 lifecycle and slice 01's immutable snapshot contract.

**Scope/output:** assign → child proposes local plan → parent returns findings or approves → child executes a bounded operation. Finalize the routed interaction schema, semantic revision projection, local owner/approver records and preparation allowlist. Define later clarification/rework operations in the same interface. Reuse the plan machinery with isolated ownership, not another scheduler.

**Escalation:** unresolved Plan v2 review blocks landing; broader user permissions require an Ask, never an approver-role workaround. Provide public fixtures for downstream matrix tests.

## Acceptance Criteria

- Before parent approval the child can only use the defined preparation allowlist; edits are denied.
- Approved child execution still passes the ordinary permission gate.
- Wrong parent, stale revision and fake textual approval are rejected; returned plans retain the same child session.
- Waiting for review holds no inference slot; progress-only updates preserve approval and material changes revoke it.
- Ordinary user-owned plans retain their existing semantics.

## Verification Plan

1. Exercise proposal, return, revision and exact-revision approval through the public runtime.
2. Verify permission denial after plan approval plus wrong-owner, stale-revision and textual-approval cases.
3. Use deterministic cancellation/admission barriers to verify waiting releases inference capacity.
4. Verify progress versus material mutation approval semantics and ordinary user-owned plan compatibility.
