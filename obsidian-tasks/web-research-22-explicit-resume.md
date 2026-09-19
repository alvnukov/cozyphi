---
id: web-research-22-explicit-resume
title: 22 — Stop research on exit and require authorized explicit continuation
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Exit/cancel stops owned work and descendants and reconciles reservations conservatively.
    - Restore does not itself start network/model work; continuation revalidates permission, binding and remaining allowance.
    - Without protected checkpoint support, sensitive state is not written; status explains reacquisition/new authorization rather than pretending resumability.
verification_plan:
    - Drive exit, crash recovery and session restore with controlled provider/process adapters.
    - Count calls before and after restore; explicit continuation tests changed grant, configuration and budget.
    - Inspect temporary/persistent artifacts for synthetic sensitive markers and run scoped lifecycle tests.
created_at: "2026-09-19T19:15:57.627934Z"
updated_at: "2026-09-19T19:15:57.627934Z"
---

## Body

**What to build:** A safe stop-and-explicit-continue lifecycle for research jobs across session/process restart.

**Blocked by:** [18](web-research-18-research-budgets.md), [21](web-research-21-origin-delivery.md).

**Contract:** [Spec](../specs/protected-web-research.md), D11–D13, Q20/Q22/Q37, T24/T27/T28. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Reuse job ownership/recovery, stop pending calls and workers, and invalidate late callbacks at shutdown. Persist sensitive checkpoints only through an already available protected backend; before 33, support explicit non-resumable status without sensitive disk state. A requested continuation checks current grant/configuration/revocation and conservative accounting. Restoring a tab alone is not authority. Keep actual checkpoint encryption owned by 33/shared storage.

**Do not change:** No plaintext recovery file, automatic post-crash retries, reset allowance under the same research identity or custom process runner.

**Proof required:** Controlled process exit/crash/restart with sink counters proving no automatic calls; explicit continuation either safely resumes authorized state or asks for a new authorized investigation. List owned process termination and reconciled reservations.

**Stop condition:** Missing shared process ownership/recovery blocks that path; do not claim cleanup from cancelling only the parent goroutine.

## Acceptance Criteria

- Exit/cancel stops owned work and descendants and reconciles reservations conservatively.
- Restore does not itself start network/model work; continuation revalidates permission, binding and remaining allowance.
- Without protected checkpoint support, sensitive state is not written; status explains reacquisition/new authorization rather than pretending resumability.

## Verification Plan

1. Drive exit, crash recovery and session restore with controlled provider/process adapters.
2. Count calls before and after restore; explicit continuation tests changed grant, configuration and budget.
3. Inspect temporary/persistent artifacts for synthetic sensitive markers and run scoped lifecycle tests.
