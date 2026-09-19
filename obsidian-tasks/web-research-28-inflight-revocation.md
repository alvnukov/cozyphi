---
id: web-research-28-inflight-revocation
title: 28 — Revoke in-flight and cached-pass results at the release boundary
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - A current host/snapshot revocation is checked at actual release, including cached passes.
    - Affected active work cancels or discards output and cannot revive via late callbacks.
    - Unknown dependency precision restricts the broader affected result rather than assuming independence.
verification_plan:
    - Arrange block-before-release, release-before-block and queued-delivery races using shared process barriers.
    - Check actual consumer delivery and sibling cancellation; verify no post-block release with stale cached pass.
    - Run only changed coordination/release race tests and record linearization expectations.
created_at: "2026-09-19T19:17:19.092625Z"
updated_at: "2026-09-19T19:17:19.092625Z"
---

## Body

**What to build:** Prevent an in-flight answer from racing a site block and escaping after revocation.

**Blocked by:** [24](web-research-24-durable-lineage.md), [27](web-research-27-host-blocks.md).

**Contract:** [Spec](../specs/protected-web-research.md), D6/D7/D9/D11, Q6/Q19/Q34, T19/T23. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Bind source dependencies and revocation versions to candidate eligibility. Use the approved shared coordination release contract so checking and committing a release cannot be separated by an unnoticed block race. Cancel affected pending calls and invalidate queued delivery/cached-pass use. Keep unaffected independent jobs working. This ticket covers not-yet-delivered outputs only; prior session context is 29.

**Do not change:** No check-only-at-job-start, boolean passed cache, warning followed by release or global cancellation of proven independent work.

**Proof required:** Controlled two-process barrier immediately before release; commit a block in the other process, then allow completion. Consumer receives zero affected content even with earlier pass; unrelated result is the positive control.

**Stop condition:** If coordination cannot linearize block/release eligibility, report that prerequisite rather than add polling sleeps.

## Acceptance Criteria

- A current host/snapshot revocation is checked at actual release, including cached passes.
- Affected active work cancels or discards output and cannot revive via late callbacks.
- Unknown dependency precision restricts the broader affected result rather than assuming independence.

## Verification Plan

1. Arrange block-before-release, release-before-block and queued-delivery races using shared process barriers.
2. Check actual consumer delivery and sibling cancellation; verify no post-block release with stale cached pass.
3. Run only changed coordination/release race tests and record linearization expectations.
