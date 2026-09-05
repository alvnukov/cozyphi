---
id: executor-context-snapshot
title: Launch a read-only child from an effective-context snapshot
status: todo
priority: high
model_level: high
task_type: feature
parent_id: plan-driven-interactive-executors
tags:
    - executors
    - plan
    - context
acceptance_criteria:
    - Full effective context and read-only overall plan arrive; unresolved tool exchanges do not.
    - The child cannot use parent grants or mutate parent state; it obtains its own editable observations.
    - Snapshot or startup failure leaves no runnable orphan; a fitting snapshot is not shortened for economy.
    - Existing unbound launches remain compatible; this slice exposes only an opt-in read-only launch.
verification_plan:
    - Use a controlled provider with macro summary, frozen microstubs, pending spawn round and fresh child observations.
    - Inject snapshot and startup failures and verify no orphan/admission leak.
    - Assert independent parent/child policies and histories and unchanged fitting input through the public runtime.
created_at: "2026-09-05T23:19:31.276064Z"
updated_at: "2026-09-05T23:19:31.276064Z"
---

## Body

**Approved slice:** 01 of [breakdown](../specs/plan-driven-executors-tickets.md). Read [contract](../specs/plan-driven-executors.md), D1–D4, before implementation.

**Blocked by:** none; use the landed child runtime and compaction interfaces.

**Capability rationale:** high — provider-valid history, snapshot ownership and authority separation require cross-module decisions. Capability is separate from reasoning effort.

**Inputs:** existing provider microprojection, macro-compacted history, retained-child startup, permission and editable-observation rules.

**Scope/output:** one opt-in read-only context launch from parent to retained child; expose snapshot metadata and actionable launch errors. Define the coherent cutoff representation and child identity projection. Preserve existing unbound launches. Local-plan approval and editing are later slices.

**Escalation:** if current provider projection cannot be reused safely, stop and propose a scoped deepening; do not build a second compaction engine. Supply the public fixtures and interfaces required by downstream tasks.

## Acceptance Criteria

- Full effective context and read-only overall plan arrive; unresolved tool exchanges do not.
- The child cannot use parent grants or mutate parent state; it obtains its own editable observations.
- Snapshot or startup failure leaves no runnable orphan; a fitting snapshot is not shortened for economy.
- Existing unbound launches remain compatible; this slice exposes only an opt-in read-only launch.

## Verification Plan

1. Use a controlled provider with macro summary, frozen microstubs, pending spawn round and fresh child observations.
2. Inject snapshot and startup failures and verify no orphan/admission leak.
3. Assert independent parent/child policies and histories and unchanged fitting input through the public runtime.
