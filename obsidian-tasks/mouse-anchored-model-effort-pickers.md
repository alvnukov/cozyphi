---
id: mouse-anchored-model-effort-pickers
title: Anchor model and effort pickers near mouse clicks
status: done
priority: medium
model_level: medium
task_type: feature
tags:
    - tui
branch: feature/mouse-anchored-model-effort-pickers
worktree_path: .worktrees/mouse-anchored-model-effort-pickers
acceptance_criteria:
    - Mouse-opened model and effort pickers appear near the click and remain within the viewport.
    - Keyboard opening remains centered.
verification_plan:
    - Run focused picker and editor tests.
    - Run formatting and scoped lint checks.
created_at: "2026-09-05T15:00:04.166063Z"
updated_at: "2026-09-05T16:15:15.41949Z"
---

## Body

Anchor model and effort pickers near their mouse trigger, retaining centered keyboard opening. Cover edge placement and mouse selection with regression tests.

**Done (2026-09-05).** Implemented in 13c464e and merged into main: mouse model/effort pickers use painted click coordinates, stay within viewport, preserve nested anchor and reset for keyboard. Added regressions and changelog. Scoped chat/palette/composer tests passed; editor picker tests and queue-test retry passed (initial package run had queue timing failure). Scoped lint: 0 issues; changed-file format and diff checks passed. External reviewers failed API 429; parent manually reviewed standards/spec. No live terminal visual check.

## Acceptance Criteria

- Mouse-opened model and effort pickers appear near the click and remain within the viewport.
- Keyboard opening remains centered.

## Verification Plan

1. Run focused picker and editor tests.
2. Run formatting and scoped lint checks.
