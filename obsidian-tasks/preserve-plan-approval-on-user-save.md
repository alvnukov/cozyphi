---
id: preserve-plan-approval-on-user-save
title: Preserve plan approval on user save
status: done
priority: medium
model_level: medium
task_type: bug
branch: bug/preserve-plan-approval-on-user-save
worktree_path: .worktrees/preserve-plan-approval-on-user-save
acceptance_criteria:
    - User save preserves existing plan approval without approving a draft.
    - Agent edits retain approval invalidation.
verification_plan:
    - Run regression and scoped plan/editor tests.
created_at: "2026-09-05T14:16:50.741638Z"
updated_at: "2026-09-05T14:25:56.159171Z"
---

## Body

Preserve existing approval when a user edits and saves a plan. Add regression coverage; do not weaken agent-edit or JIT approval.

**Done (2026-09-05).** Implemented in 2aa8526 and merged into main: trusted UI plan saves preserve existing approval without approving drafts; model patch policy and JIT contract epochs remain unchanged. Regression tests cover returned/durable/published state, stale saves, and model reapproval. Scoped agent/controller/session/plangate tests pass; scoped lint reports 0 issues; standards/spec review found no blocking issues. One busy-period test timed out under concurrent runs, then passed three isolated repetitions and the package rerun.

## Acceptance Criteria

- User save preserves existing plan approval without approving a draft.
- Agent edits retain approval invalidation.

## Verification Plan

1. Run regression and scoped plan/editor tests.
