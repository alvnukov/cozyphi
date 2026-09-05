---
id: session-local-shell-ownership
title: Bind local shell cwd and cancellation to its retained session
status: done
priority: high
model_level: medium
task_type: bug
parent_id: multisession-mode
tags:
    - multisession
    - ownership
acceptance_criteria:
    - User !cmd uses its retained View's canonical workspace cwd, never ambient process cwd.
    - Admission is synchronous; immediate cancellation/close cannot miss an accepted local shell or admit a second one.
    - Public runner tests cover cwd and cancellation ownership; integrate with retained Views.
verification_plan:
    - Scoped public submit runner tests and race checks.
    - Verify View disposal cancels its local shell without touching siblings.
created_at: "2026-09-05T17:00:24.951404Z"
updated_at: "2026-09-05T17:42:58.703574Z"
---

## Body

Observed during retained View investigation: internal/tui/submit/bash.go calls ExecShell without cwd; running flag and cancel function are installed only after launching the goroutine, and no owned Close path exists. This breaks explicit child workdirs and permits immediate duplicate/cancel races. Fix inside the multisession-registry worktree prerequisite, not as a second session queue.

**Done (2026-09-05).** Implemented within parent multisession-registry worktree, landed in 3dde2b4 and main. Explicit canonical cwd via existing tooldef.WithCwd seam; admission/context installed synchronously; duplicates refused; Cancel immediate; Close terminal and waits actual exit/final publication, retaining ownership on deadline. View uses explicit cwd and owns runner disposal. Scoped submit race tests and post-lint-fix regressions passed. No separate worktree or push.

## Acceptance Criteria

- User !cmd uses its retained View's canonical workspace cwd, never ambient process cwd.
- Admission is synchronous; immediate cancellation/close cannot miss an accepted local shell or admit a second one.
- Public runner tests cover cwd and cancellation ownership; integrate with retained Views.

## Verification Plan

1. Scoped public submit runner tests and race checks.
2. Verify View disposal cancels its local shell without touching siblings.
