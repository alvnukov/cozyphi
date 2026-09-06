---
id: multisession-sessions-panel
title: 'Deferred: optional grouped session list beyond V1 tabs'
status: blocked
priority: low
model_level: high
task_type: feature
parent_id: multisession-mode
tags:
    - cozyphi
    - tui
    - multisession
branch: feature/multisession-sessions-panel
worktree_path: .worktrees/multisession-sessions-panel
acceptance_criteria:
    - Before implementation, a new user-approved scope explains why a grouped sidebar or searchable list is needed beyond the V1 tab selector; this task is not implicitly authorized by V1.
    - If selected, the supplemental UI reuses live session identity, project labels, statuses and open/close/navigation routes rather than introducing another registry or authority path.
    - The selected design works on narrow terminals and through keyboard/mouse without stealing composer or modal input; status is not color-only and selection is distinct from unread.
    - Only the newly approved presentation and persistence behavior is implemented and documented; recent-history/restore and new project-opening work are linked to their owning scope, not duplicated.
verification_plan:
    - At selection time, validate the new design against tab UX and remaining user needs before creating runtime changes; update these provisional criteria with the approved shape.
    - Use controlled retained sessions to check filter/group order, live target identity, truncation, narrow layout and keyboard/mouse routing without providers.
    - Run scoped render/dispatch tests for the actual changed packages, not a presumed new sessionpane package; one scoped lint at most. Do not run Go gates for this deferral note.
created_at: "2026-09-04T07:31:55.427967Z"
updated_at: "2026-09-06T14:03:16.865328Z"
---

## Body

**Disposition (2026-09-06).** Deferred optional extension, outside the approved tab-based V1. The user chose tabs with one visible conversation, not a mandatory left sidebar. Keep this existing task as the home of the earlier idea; do not delete its intent or create a duplicate panel ticket. It must not block project opening, hotkeys, attention, lifecycle or V1 hardening.

**Earlier proposal retained for a future decision, not current implementation instructions.** A grouped project→session list with titles, status/job/unread marks, filter, project folding, keyboard focus, rename/close/open actions and mouse selection. The previous mockup used a 28-column draggable sidebar with width/visibility persistence and an overlay below 100 columns. These exact dimensions, bindings, color palette and persistence fields are unapproved future choices, not existing runtime facts. A searchable overlay may satisfy the need without a permanent sidebar; choose the minimum design when this work is selected.

**Reuse rather than duplicate.** Existing Registry/View/selector already retain tabs and expose navigation/status. Read project identity and title from the owning session; stable live IDs target actions even when filtering changes row positions. Sidebar grouping must not change the numerical meaning of /switch N or manufacture a new session order. Existing title pinning and permission/close confirmation remain authoritative.

**Future coordination.** The initial /new <path> and minimal project label belong to [multisession-projects](multisession-projects.md). Navigation/help belong to [multisession-hotkeys](multisession-hotkeys.md); attention and cues retain their own scope. Recent projects, cross-project history listing and saved tab sets require an explicit later selection rather than being pulled in through this presentation task.

**Blocked by:** [multisession-hardening](multisession-hardening.md), plus a new user UX/scope decision after V1. This is intentionally not a reverse prerequisite of any V1 delivery. A completed V1 does not by itself approve the sidebar.

**Blocked (2026-09-06).** Deferred outside approved tab-based V1. Resume only after multisession-hardening and a fresh user decision selecting a supplemental grouped-list/sidebar scope. No V1 task depends on this optional extension.

## Acceptance Criteria

- Before implementation, a new user-approved scope explains why a grouped sidebar or searchable list is needed beyond the V1 tab selector; this task is not implicitly authorized by V1.
- If selected, the supplemental UI reuses live session identity, project labels, statuses and open/close/navigation routes rather than introducing another registry or authority path.
- The selected design works on narrow terminals and through keyboard/mouse without stealing composer or modal input; status is not color-only and selection is distinct from unread.
- Only the newly approved presentation and persistence behavior is implemented and documented; recent-history/restore and new project-opening work are linked to their owning scope, not duplicated.

## Verification Plan

1. At selection time, validate the new design against tab UX and remaining user needs before creating runtime changes; update these provisional criteria with the approved shape.
2. Use controlled retained sessions to check filter/group order, live target identity, truncation, narrow layout and keyboard/mouse routing without providers.
3. Run scoped render/dispatch tests for the actual changed packages, not a presumed new sessionpane package; one scoped lint at most. Do not run Go gates for this deferral note.
