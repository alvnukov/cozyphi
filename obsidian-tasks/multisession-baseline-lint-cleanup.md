---
id: multisession-baseline-lint-cleanup
title: Clean up three pre-existing UI lint findings
status: done
priority: low
model_level: low
task_type: chore
tags:
    - lint
branch: chore/multisession-baseline-lint-cleanup
worktree_path: .worktrees/multisession-baseline-lint-cleanup
acceptance_criteria:
    - Remove the three reported ineffassign/unparam findings without changing UI behavior.
verification_plan:
    - Verify findings still exist on current main and check parallel ownership.
    - Run affected UI tests and normal lint gate in the dedicated task.
created_at: "2026-09-05T20:48:57.631048Z"
updated_at: "2026-09-06T17:35:48.436846Z"
---

## Body

The single child-session lint run reported three findings outside the child diff: internal/components/palette/command_palette.go:529 ineffectual visible=1; internal/tui/sidebar/sidebar_plan_test.go:74 drawWide height always40; internal/tui/submit/bash.go:239 showToast duration always3s. These files are unchanged by the interactive-child core slice. Evidence /tmp/cozyphi-child-final-lint.log. Keep isolated from parallel UI work and verify after reconciling main.

**Done (2026-09-06).** Closed without code changes: verification on 2026-09-07 against main (798c8e6) found all three findings already fixed — command_palette.go recomputes visible from the final boxH (ineffectassign gone), drawWide no longer takes a height parameter (unparam gone), showToast uses the named constant bashToastDuration (unparam gone). Fixes landed in ec728b6 «fix(ci): repair red main — stale assertion, test race, lint debt». No worktree or branch was created; no Go gates run (no .go diff).

## Acceptance Criteria

- Remove the three reported ineffassign/unparam findings without changing UI behavior.

## Verification Plan

1. Verify findings still exist on current main and check parallel ownership.
2. Run affected UI tests and normal lint gate in the dedicated task.
