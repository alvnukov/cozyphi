---
id: executor-assignment-recovery
title: Recover executor review relationships without resurrecting work
status: blocked
priority: high
model_level: high
task_type: feature
parent_id: plan-driven-interactive-executors
tags:
    - executors
    - recovery
    - lifecycle
acceptance_criteria:
    - Restore preserves assignment, local-plan review and result-review relationships without inventing running or accepted states.
    - Human-stop protection persists; explicit resume validates current authority and creates a new attempt.
    - Corrupt or unknown records are preserved and reported.
    - Closing preserves disk history; synchronization does not reopen a closed View.
    - Recovery respects live foreign-process ownership.
verification_plan:
    - Crash/restart at proposal, approval, result receipt and acknowledgement boundaries.
    - Test a live foreign-process owner and a closed View during synchronization.
    - Test corrupt records, missing worktree and explicit human resume with a new attempt.
created_at: "2026-09-05T23:21:16.172031Z"
updated_at: "2026-09-05T23:21:16.172031Z"
---

## Body

**Approved slice:** 08 of [breakdown](../specs/plan-driven-executors-tickets.md). Read [contract](../specs/plan-driven-executors.md), D8–D9.

**Blocked by:** executor-human-authority, multisession-lifecycle-restore, job-recovery-process-ownership.

**Capability rationale:** high — durable ownership, stop latches and process liveness must agree across restarts; this is not serialization-only work.

**Inputs:** slices 02–03 durable schemas, slice 05 stop/intervention state and accepted restore/process-ownership behavior.

**Scope/output:** recover assignment/plan-review/result-review relationships when inspecting or explicitly resuming retained history. Integrate executors into existing close/restore; do not duplicate its UI or create a second history index.

**Escalation:** unresolved lifecycle/process-ownership prerequisites remain blockers; do not substitute best-effort process killing or fabricated success.

## Acceptance Criteria

- Restore preserves assignment, local-plan review and result-review relationships without inventing running or accepted states.
- Human-stop protection persists; explicit resume validates current authority and creates a new attempt.
- Corrupt or unknown records are preserved and reported.
- Closing preserves disk history; synchronization does not reopen a closed View.
- Recovery respects live foreign-process ownership.

## Verification Plan

1. Crash/restart at proposal, approval, result receipt and acknowledgement boundaries.
2. Test a live foreign-process owner and a closed View during synchronization.
3. Test corrupt records, missing worktree and explicit human resume with a new attempt.
