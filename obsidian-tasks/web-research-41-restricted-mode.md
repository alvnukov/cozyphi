---
id: web-research-41-restricted-mode
title: 41 — Gate restricted runtime readiness on all mandatory web protections
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Aggregate runtime eligibility requires actual isolation/disablement AND delivered budgets/account admission, lifecycle/action grants, incidents/revocation and human resolution for enabled paths.
    - Every supported platform reports enabled/disabled/unproved capability; missing core barrier or mandatory integration disables web itself.
    - General off/observe/allow-all cannot restore unsafe channels; format-specific safety/revocation must be verified before enabling that adapter.
verification_plan:
    - Remove each mandatory shared/web capability separately, including budget, scheduler, lineage/grants, incidents/revocation and user resolution; assert zero production dispatch.
    - On each supported platform attempt actual cache/file/shell/MCP/hook/watch/child paths under off/observe/allow-all and inspect controlled sinks.
    - Verify missing optional adapter proofs keep only those methods unavailable, existing processes are reconciled or block readiness, and scoped evidence never claims unrun platforms passed.
created_at: "2026-09-19T19:21:47.871023Z"
updated_at: "2026-09-19T19:34:10.242186Z"
---

## Body

**What to build:** The aggregate fail-closed runtime eligibility decision for the assembled restricted protected mode. Earlier integration tickets, especially 11, do not independently enable production research.

**Blocked by:** [01](web-research-01-channel-inventory.md), [02](web-research-02-not-ready.md), [19](web-research-19-account-admission.md), [22](web-research-22-explicit-resume.md), [25](web-research-25-post-web-grants.md), [29](web-research-29-delivered-revocation.md), [31](web-research-31-user-resolution.md), [32](web-research-32-circuit-breaker.md).

**Unresolved prerequisite:** Approved and landed platform isolation/disablement implementations for every enabled channel, not merely design. Record concrete IDs/proof; no all-platform sandbox implementation belongs in this medium slice.

**Contract:** [Spec](../specs/protected-web-research.md), D2/D7/D15/D16, Q11/Q13/Q25/Q26/Q39, T02/T12/T13/T30/T31. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Integrate existing delivered capabilities into host-owned eligibility, refusing if ANY required protection is unavailable. Dependencies provide common budgets, pinned account queue, owned lifecycle, durable effective-action enforcement, global incidents/block/revocation and user resolution. Use shared capability policy at advertisement AND execution for tools, file access, prehooks/hooks, MCP, watches and children. Safely reconcile already-running unmediated processes or refuse enablement. Optional format/storage methods require their own completed contract/proof before enabling: e.g. renderer/image derivation revocation requires 40, persistent storage requires 33/34/35. Missing such methods may leave a clearly limited static/nonpersistent mode, never weaken core safety. This runtime eligibility is necessary, not sufficient for rollout: consented evaluation 45 and readiness evidence 46 still determine accepted full delivery.

**Do not change:** No default-true readiness, command allowlist as sandbox, weaker platform guarantee, duplicate process manager or unapproved rollout.

**Proof required:** Independently remove each mandatory capability and observe unavailable web with zero production calls. Per-platform/channel sink matrix demonstrates actual disablement, one permitted safe control and denial of unready optional adapters. Passing 11 alone must not enable research.

**Stop condition:** Unproved enabled route or missing mandatory integration blocks eligibility; absent test hardware cannot prove safety.

## Acceptance Criteria

- Aggregate runtime eligibility requires actual isolation/disablement AND delivered budgets/account admission, lifecycle/action grants, incidents/revocation and human resolution for enabled paths.
- Every supported platform reports enabled/disabled/unproved capability; missing core barrier or mandatory integration disables web itself.
- General off/observe/allow-all cannot restore unsafe channels; format-specific safety/revocation must be verified before enabling that adapter.

## Verification Plan

1. Remove each mandatory shared/web capability separately, including budget, scheduler, lineage/grants, incidents/revocation and user resolution; assert zero production dispatch.
2. On each supported platform attempt actual cache/file/shell/MCP/hook/watch/child paths under off/observe/allow-all and inspect controlled sinks.
3. Verify missing optional adapter proofs keep only those methods unavailable, existing processes are reconciled or block readiness, and scoped evidence never claims unrun platforms passed.
