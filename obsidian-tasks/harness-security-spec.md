---
id: harness-security-spec
title: Specify opt-in harness security and publish the approved delivery plan
status: in_progress
priority: high
model_level: high
task_type: docs
parent_id: harness-security-hardening
tags: [security, design, planning]
branch: docs/harness-security-spec
worktree_path: .worktrees/harness-security-spec
acceptance_criteria:
  - A standalone English specification preserves every approved mode, authority, trust, storage and rollout decision without claiming implementation.
  - Thirteen approved delivery slices have explicit dependencies, acceptance criteria and scoped public-interface verification plans.
  - Existing dataflow and eval tasks are reused; the parent epic and unrelated notes remain unchanged.
  - Only owned documentation and registry changes are signed and published on an isolated branch through a PR; main is untouched.
  - Reviewer or user merge is confirmed before this planning task is marked done; implementation tasks remain open.
verification_plan:
  - Validate owned markdown links, YAML fields, mirrored criteria, task references, acyclic dependencies and frontier statuses.
  - Review specification-to-ticket coverage and the diff for unsupported claims, secrets, code changes and unrelated files.
  - Verify the signed commit, PR base/head and actual CI status; run no Go gates for this documentation-only change.
created_at: "2026-09-09T11:16:39Z"
updated_at: "2026-09-09T11:16:39Z"
---

## Body

**What to build:** Publish the completed interview and architecture as a standalone [specification](../specs/harness-security.md), [approved ticket map](../specs/harness-security-tickets.md) and individual registry notes. No implementation, model trials or merge are authorized by this planning task.

**Blocked by:** None — can start immediately.

**Started (2026-09-09):** Working on branch `docs/harness-security-spec` in the isolated task worktree, based on upstream commit 5181f3dd3cbb085e390d375eb8baeaf31d3acb11. The user approved the 13-slice breakdown, dependencies and public testing seams. Existing main-checkout changes are outside this work.

**Registry limitation and authorization (2026-09-09):** The current native task tool cannot target this worktree. The user explicitly approved direct writes of the standard task markdown format only in the project branch. Native task is used read-only; all ledger changes travel with this PR rather than landing on main.

**Verification (2026-09-09):** Owned-scope validation passed for 14 task notes, mirrored criteria/verification, 54 local document links, the approved acyclic 13-slice DAG and its frontier statuses, 32 stories and D1–D9/S01–S17 coverage. Independent read-only review prompted explicit human storage-design acceptance and per-slice headless scenarios. No Go checks or model trials were run; no runtime behavior changed.

**Delivery state:** Documents validated and ready for PR review; publication and CI state are recorded on the PR for `docs/harness-security-spec`. Keep this task in progress through review. A local commit or open PR does not complete the delivery, and no implementation task is closed by the specification.

## Acceptance Criteria

- A standalone English specification preserves every approved mode, authority, trust, storage and rollout decision without claiming implementation.
- Thirteen approved delivery slices have explicit dependencies, acceptance criteria and scoped public-interface verification plans.
- Existing dataflow and eval tasks are reused; the parent epic and unrelated notes remain unchanged.
- Only owned documentation and registry changes are signed and published on an isolated branch through a PR; main is untouched.
- Reviewer or user merge is confirmed before this planning task is marked done; implementation tasks remain open.

## Verification Plan

1. Validate owned markdown links, YAML fields, mirrored criteria, task references, acyclic dependencies and frontier statuses.
2. Review specification-to-ticket coverage and the diff for unsupported claims, secrets, code changes and unrelated files.
3. Verify the signed commit, PR base/head and actual CI status; run no Go gates for this documentation-only change.
