---
id: routing-children-lifecycle
title: 06 — Route children and recover safely across plan transitions
status: blocked
priority: high
model_level: very_high
task_type: feature
parent_id: subscription-aware-routing
tags:
    - routing
    - subscriptions
    - agents
acceptance_criteria:
    - Main and interactive/headless children use the same account admission and interpretation contracts at launch, step transition and unavailability, including explicit start, implicit start, resume, settle, restore and plan completion.
    - Children inherit required tier and user priority/authority from the originating round, not opportunistically upgraded actual tier; delayed launch reserves against current account evidence.
    - Selection, effective native effort and authoritative tier metadata form one immutable inference/tool-round snapshot; post-switch metadata arrives before the next inference and stale statements are not authoritative after restore/compaction.
    - Subsequent chargeable calls renew admission without switching profiles every turn or replaying already-applied tool effects.
    - Quota exhaustion, overload, throttling, authentication failure, unsupported settings and context incompatibility produce distinct bounded/cancellable outcomes; stale pins never silently inherit.
    - Existing context-size/compaction approval and child execution gates remain intact; UI and headless outputs distinguish pending/effective selection and actionable waiting.
verification_plan:
    - Exercise S03, S15–S17 and S20 with synthetic transports and real engine/child preparation seams.
    - Cover explicit/implicit start, resume, settle, restored active step, completion and interactive/headless children.
    - Inject model changes during a tool round and delayed spawn; assert origin intent, current budget claim and pre-inference metadata agree.
    - Simulate partial streams, overload/auth/quota failures, cancellation and context overflow; assert bounded retry and no duplicated tool effects.
created_at: "2026-09-06T11:11:40.017394Z"
updated_at: "2026-09-06T11:18:03.794834Z"
---

## Body

**Approved slice:** 06 of the [breakdown](../specs/subscription-aware-routing-tickets.md). Read the [contract](../specs/subscription-aware-routing.md), D2 and D7.

**Blocked by:** routing-openai-account-admission.

**What to build:** Extend the working main route through child launches and every safe plan lifecycle transition, including recovery, without mutating an in-flight round or losing inherited intent.

**Inputs:** Versioned profile metadata and shared account admission. Existing engine snapshot/child preparation paths are the integration seam.

**Scope/output:** Complete main/child lifecycle behavior, admission/reconciliation for all inference attempts, correct first/post-switch prompts, bounded unavailability recovery and visible state.

**Escalation:** Tier routing is not authority to compact away requirements or override executor-context-budget consent. Do not replay tool side effects after a partial stream or silently resolve a stale human pin.

**Blocked (2026-09-06).** Waiting for routing-openai-account-admission; extend its verified main path through child/lifecycle execution.

**Note (2026-09-06).** Review clarification: S03 explicitly combines delayed child launch with grant expiry or revocation before dispatch. Preserve originating required tier/priority, but revalidate current grant validity before inference; an old round snapshot must not resurrect stale consent. Include this combined adversarial case in lifecycle verification.

## Acceptance Criteria

- Main and interactive/headless children use the same account admission and interpretation contracts at launch, step transition and unavailability, including explicit start, implicit start, resume, settle, restore and plan completion.
- Children inherit required tier and user priority/authority from the originating round, not opportunistically upgraded actual tier; delayed launch reserves against current account evidence.
- Selection, effective native effort and authoritative tier metadata form one immutable inference/tool-round snapshot; post-switch metadata arrives before the next inference and stale statements are not authoritative after restore/compaction.
- Subsequent chargeable calls renew admission without switching profiles every turn or replaying already-applied tool effects.
- Quota exhaustion, overload, throttling, authentication failure, unsupported settings and context incompatibility produce distinct bounded/cancellable outcomes; stale pins never silently inherit.
- Existing context-size/compaction approval and child execution gates remain intact; UI and headless outputs distinguish pending/effective selection and actionable waiting.

## Verification Plan

1. Exercise S03, S15–S17 and S20 with synthetic transports and real engine/child preparation seams.
2. Cover explicit/implicit start, resume, settle, restored active step, completion and interactive/headless children.
3. Inject model changes during a tool round and delayed spawn; assert origin intent, current budget claim and pre-inference metadata agree.
4. Simulate partial streams, overload/auth/quota failures, cancellation and context overflow; assert bounded retry and no duplicated tool effects.
