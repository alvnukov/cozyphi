---
id: multisession-interactive-children
title: Retain interactive child sessions with turn-scoped interruption
status: todo
priority: high
model_level: high
task_type: feature
parent_id: multisession-mode
tags:
    - multisession
    - agents
acceptance_criteria:
    - Native interactive children retain a Controller/Session and reuse the multisession registry; headless runner remains compatible.
    - SessionID/JobID/TurnID and parent linkage are structured; interrupt cancels one turn, while leave-after-interruption stops the assignment once without discarding history.
    - Queued input, permissions and continuation stay origin-scoped; role ceilings, cancellation and concurrency limits hold in race tests.
verification_plan:
    - Public-interface lifecycle tests with controlled orderings for interrupt/submit/leave/finish and queued text-only turns.
    - Cross-session and stale interaction rejection; read-only roles, cancellation, timeout and resource budgets.
    - Focused agent/job/controller tests and race checks; existing headless spawn/wait/cancel compatibility.
created_at: "2026-09-05T15:19:15.031841Z"
updated_at: "2026-09-05T15:19:15.031841Z"
---

## Body

Implement the approved lifecycle contract in obsidian-tasks/interactive-child-sessions-design.md. No separate session registry or prompt scheduler. Native interactive and headless runners form the real adapter seam. Serialize submit, interrupt, leave and finish, prevent late revival, and retain terminal results independently from conversation history. **Blocked by:** multisession-runtime-split, multisession-registry. **Effort:** high (maximum supported). Code only in the task worktree.

## Acceptance Criteria

- Native interactive children retain a Controller/Session and reuse the multisession registry; headless runner remains compatible.
- SessionID/JobID/TurnID and parent linkage are structured; interrupt cancels one turn, while leave-after-interruption stops the assignment once without discarding history.
- Queued input, permissions and continuation stay origin-scoped; role ceilings, cancellation and concurrency limits hold in race tests.

## Verification Plan

1. Public-interface lifecycle tests with controlled orderings for interrupt/submit/leave/finish and queued text-only turns.
2. Cross-session and stale interaction rejection; read-only roles, cancellation, timeout and resource budgets.
3. Focused agent/job/controller tests and race checks; existing headless spawn/wait/cancel compatibility.
