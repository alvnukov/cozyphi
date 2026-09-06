---
id: routing-priority-scheduling
title: 05 — Schedule important, active and background work consistently
status: blocked
priority: high
model_level: high
task_type: feature
parent_id: subscription-aware-routing
tags:
    - routing
    - subscriptions
acceptance_criteria:
    - Admission orders explicitly important work before ordinary active work before ordinary background work across local processes and children.
    - Active identity follows the latest user assignment with consistent cross-process ordering, never UI focus; priority inherits from session and user step/job overrides are honored.
    - The model may recommend higher priority but cannot self-promote or forge assignment/activity authority.
    - An active-work upgrade may postpone background work even at its minimum required tier; background latency permits stronger useful upgrades when admitted.
    - Priority affects future admission, not forced cancellation of an in-flight round; waits are cancellable, wake on relevant changes and expose starvation rather than silently invert priority.
    - User-visible queue/status explanations include priority source, reserve/surplus reason and pending selection without exposing other assignments' content.
verification_plan:
    - Run S10, S11 and S19 with two processes and main/child queues under scarce and surplus conditions.
    - Change UI focus without a new assignment and assert unchanged priority; submit new work and assert consistent activity ordering.
    - Demonstrate active upgrade delaying background required-tier work, then cancellation and wake-up after budget/reset change.
    - Attempt model-authored priority escalation and verify rejection; inspect starvation and privacy-safe explanations.
created_at: "2026-09-06T11:12:00.239065Z"
updated_at: "2026-09-06T11:15:04.390398Z"
---

## Body

**Approved slice:** 05 of the [breakdown](../specs/subscription-aware-routing-tickets.md). Read the [contract](../specs/subscription-aware-routing.md), D4–D5.

**Blocked by:** routing-adaptive-reserve, routing-children-lifecycle.

**What to build:** Schedule useful main/child work under the agreed importance/activity policy, including the explicitly approved trade-off that an active upgrade can delay ordinary background minimum-quality execution.

**Inputs:** Adaptive reserve policy, child inheritance and cross-process admission.

**Scope/output:** User priority controls, assignment-based activity identity, coordinated waiting/admission, status explanations and deterministic contention traces. Within-priority ordering may age; cross-priority inversion is not authorized.

**Escalation:** Sustained starvation is a visible user choice/problem, not permission to cancel work or redefine activity as keyboard focus.

**Blocked (2026-09-06).** Waiting for routing-adaptive-reserve and routing-children-lifecycle; coordinated queues need both reserve decisions and child intent inheritance.

## Acceptance Criteria

- Admission orders explicitly important work before ordinary active work before ordinary background work across local processes and children.
- Active identity follows the latest user assignment with consistent cross-process ordering, never UI focus; priority inherits from session and user step/job overrides are honored.
- The model may recommend higher priority but cannot self-promote or forge assignment/activity authority.
- An active-work upgrade may postpone background work even at its minimum required tier; background latency permits stronger useful upgrades when admitted.
- Priority affects future admission, not forced cancellation of an in-flight round; waits are cancellable, wake on relevant changes and expose starvation rather than silently invert priority.
- User-visible queue/status explanations include priority source, reserve/surplus reason and pending selection without exposing other assignments' content.

## Verification Plan

1. Run S10, S11 and S19 with two processes and main/child queues under scarce and surplus conditions.
2. Change UI focus without a new assignment and assert unchanged priority; submit new work and assert consistent activity ordering.
3. Demonstrate active upgrade delaying background required-tier work, then cancellation and wake-up after budget/reset change.
4. Attempt model-authored priority escalation and verify rejection; inspect starvation and privacy-safe explanations.
