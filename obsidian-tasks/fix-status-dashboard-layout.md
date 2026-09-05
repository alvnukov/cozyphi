---
id: fix-status-dashboard-layout
title: Correct status Config and Stats to match dashboard reference
status: done
priority: high
model_level: high
task_type: bug
tags:
    - tui
    - status
    - visual
branch: bug/fix-status-dashboard-layout
worktree_path: .worktrees/fix-status-dashboard-layout
acceptance_criteria:
    - Config is a read-only allowlisted effective-settings summary, not an embedded settings editor.
    - 'Stats Overview follows the supplied screenshot: weekly calendar with weekday/month labels and intensity legend, visible Overview/Models and period selectors, two-column metrics and token breakdown.'
    - All values use real observations with explicit unavailable/partial states; existing close-count preference and async lifecycle remain correct.
    - Rendering, navigation and resize tests plus scoped gates and review pass; changes merged to main and task worktree/branch cleaned.
verification_plan:
    - Compare current rendering and available metrics against the supplied screenshot.
    - Add rendered-surface assertions for layout, color intensity, empty/partial data and terminal sizes; regression-test read-only Config and navigation.
    - Run formatting only on changed files, scoped lint and tests/race for affected packages; review diff against visual contract.
    - Commit and merge to main, preserve unrelated work, remove clean task worktree/branch, then close ledger.
created_at: "2026-09-05T13:09:47.266091Z"
updated_at: "2026-09-05T13:49:32.160932Z"
---

## Body

Correct the merged status dashboard after user visual review. The current Config embeds /settings incorrectly; replace it with a read-only effective-settings summary. Stats must follow the newly supplied screenshot rather than a textual approximation: Overview/Models subnavigation; a roughly year-wide weekly calendar with seven day rows, month labels, Mon/Wed/Fri labels and Less/More intensity legend; All time/Last 7 days/Last 30 days selector; two-column favorite model/tokens, sessions/longest session, active days/longest streak, most active day/current streak, then token breakdown and footer. Derive only defensible metrics, explicitly mark unavailable values and incomplete coverage. Do not copy the screenshot's numbers, Claude branding or arbitrary literary comparison. Preserve existing provider changes, preference semantics and permission/data ownership boundaries. Scope changes to an isolated worktree and complete merge before closing.

**Note (2026-09-05).** Implemented correction in isolated worktree, code commit 2111dff. Config is detached/read-only with unresolved effective values explicit; Stats uses colored week-column calendar, period/coverage states, paired metrics and token breakdown. Standards and Spec findings corrected and re-reviewed; scoped format/lint (0 issues), tests and race passed. Merged latest main into task branch without conflict; final integration gates running before delivery to main and cleanup. Not done yet.

**Done (2026-09-05).** Delivered to main: implementation 2111dff, integration ea1e46f, merge ad1b6c0; ancestry verified. Task worktree and bug/fix-status-dashboard-layout deleted. Config read-only allowlist, Stats positioned orange calendar/Overview/Models/periods/paired metrics/token breakdown, unavailable/partial/excluded coverage explicit; midnight and 80x24/40x24 regressions covered. Standards/Spec review findings corrected and rechecked. Scoped formatting, lint 0 issues, tests and race across statuspane/editor/controller/settings/session/project/commands passed again after integrating current main (including Codex changes). Surface character/color/coordinate tests and preview verified; no claim of interactive user visual approval. Foreign go.sum and untracked task files unchanged by SHA-256; no push.

## Acceptance Criteria

- Config is a read-only allowlisted effective-settings summary, not an embedded settings editor.
- Stats Overview follows the supplied screenshot: weekly calendar with weekday/month labels and intensity legend, visible Overview/Models and period selectors, two-column metrics and token breakdown.
- All values use real observations with explicit unavailable/partial states; existing close-count preference and async lifecycle remain correct.
- Rendering, navigation and resize tests plus scoped gates and review pass; changes merged to main and task worktree/branch cleaned.

## Verification Plan

1. Compare current rendering and available metrics against the supplied screenshot.
2. Add rendered-surface assertions for layout, color intensity, empty/partial data and terminal sizes; regression-test read-only Config and navigation.
3. Run formatting only on changed files, scoped lint and tests/race for affected packages; review diff against visual contract.
4. Commit and merge to main, preserve unrelated work, remove clean task worktree/branch, then close ledger.
