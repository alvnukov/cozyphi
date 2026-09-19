---
id: web-research-08-release-races
title: 08 — Make parallel release and cancellation races deterministic
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Safety-first and extraction-first completion never release before both required passes and final pass.
    - Rejection/cancellation stops or discards siblings; late and duplicate callbacks cannot restart or deliver a result.
    - Account reservations and owned goroutines finish on all tested exit paths.
verification_plan:
    - Drive both completion orders and rejection/cancel races with channels/barriers and no wall-clock sleeps.
    - Assert no intermediate parent tokens, at-most-once terminal result, canceled siblings and no leaked reservations.
    - Run go test -race only for changed concurrency packages; record exact commands and controlled event traces.
created_at: "2026-09-19T19:11:59.148626Z"
updated_at: "2026-09-19T19:11:59.148626Z"
---

## Body

**What to build:** Deterministic concurrency correctness for the one-source job under controlled completion order.

**Blocked by:** [06](web-research-06-single-source-tracer.md).

**Contract:** [Spec](../specs/protected-web-research.md), D5/D6/D12/D13, Q15/Q23/Q24, T03/T04/T26/T27. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Use barriers in controlled provider calls to arrange safety-first, extraction-first, source rejection while its sibling works, cancellation before final check, and late/duplicate completion. Make terminal job state monotonic using the existing lifecycle contract. Honor shared account admission: concurrent eligibility does not promise provider capacity. Test cleanup through observable job completion, canceled calls and released reservations rather than private lock layouts.

**Do not change:** No sleep-based synchronization, account scheduler rewrite or production networking. Do not serialise safety then extraction to make races disappear.

**Proof required:** Event order from deterministic tests showing both initial calls eligible, no early output, exactly one terminal delivery at most, and no orphan work after cancellation. Attach changed-package race-test results.

**Stop condition:** A provider API that cannot honor cancellation needs an explicit bounded discard/lifetime contract; never treat late success as renewed authority.

## Acceptance Criteria

- Safety-first and extraction-first completion never release before both required passes and final pass.
- Rejection/cancellation stops or discards siblings; late and duplicate callbacks cannot restart or deliver a result.
- Account reservations and owned goroutines finish on all tested exit paths.

## Verification Plan

1. Drive both completion orders and rejection/cancel races with channels/barriers and no wall-clock sleeps.
2. Assert no intermediate parent tokens, at-most-once terminal result, canceled siblings and no leaked reservations.
3. Run go test -race only for changed concurrency packages; record exact commands and controlled event traces.
