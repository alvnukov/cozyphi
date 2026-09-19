---
id: web-research-18-research-budgets
title: 18 — Bound total research work across stages, retries and continuation
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - One research scope bounds time, calls, pages, bytes, regions, output, queue/concurrency, retries and continuations.
    - Every required stage and chargeable failed/cancelled attempt consumes the applicable allowance; repeated tool calls do not reset it.
    - Exhaustion stops unchecked work and returns only an already-checkable partial/stopped state; extensions require user authority.
verification_plan:
    - Use fake clock and provider/account counters to hit each configured bound exactly and one unit beyond.
    - Retry, cancel and continue the same scope; verify stable counters and no unchecked result on final-stage exhaustion.
    - Run scoped budget/lifecycle tests and attach the accounting table with measured versus unknown fields distinguished.
created_at: "2026-09-19T19:14:42.000699Z"
updated_at: "2026-09-19T19:14:42.000699Z"
---

## Body

**What to build:** A hard research budget shared by all work caused by one authorized investigation.

**Blocked by:** [06](web-research-06-single-source-tracer.md).

**Contract:** [Spec](../specs/protected-web-research.md), D12/D13, Q12/Q22/Q37, T27/T28/T29. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Bind counters/deadlines to existing research identity and grant, including retries and continuations. Use controlled clock/admission seams and checked arithmetic. Reserve enough budget before each stage; do not release a candidate merely because final checking no longer fits. Track actual/estimated/unknown provider usage separately. Configuration exposes hard bounds, but measured defaults are deferred to evaluation; test values are explicitly fixtures.

**Do not change:** No parallel account scheduler, invented remaining subscription quota, unlimited unknown budget, silent scope reset or user authority minted by a model.

**Proof required:** Table of tiny explicit budgets showing each stage charged, refusal at the exact boundary, no over-budget dispatch and no reset after continuation/repeated calls. Include cancel/retry and final-check exhaustion.

**Stop condition:** Undefined charge semantics must be reported as unknown and conservatively bounded, not fabricated.

## Acceptance Criteria

- One research scope bounds time, calls, pages, bytes, regions, output, queue/concurrency, retries and continuations.
- Every required stage and chargeable failed/cancelled attempt consumes the applicable allowance; repeated tool calls do not reset it.
- Exhaustion stops unchecked work and returns only an already-checkable partial/stopped state; extensions require user authority.

## Verification Plan

1. Use fake clock and provider/account counters to hit each configured bound exactly and one unit beyond.
2. Retry, cancel and continue the same scope; verify stable counters and no unchecked result on final-stage exhaustion.
3. Run scoped budget/lifecycle tests and attach the accounting table with measured versus unknown fields distinguished.
