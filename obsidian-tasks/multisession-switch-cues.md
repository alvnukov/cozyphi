---
id: multisession-switch-cues
title: Verify project identity and input ownership when switching tabs
status: blocked
priority: high
model_level: high
task_type: feature
parent_id: multisession-mode
tags:
    - cozyphi
    - tui
    - ux
    - multisession
branch: feature/multisession-switch-cues
worktree_path: .worktrees/multisession-switch-cues
acceptance_criteria:
    - The existing tab selector and active-session cues unambiguously identify session and project, including two projects with identical directory basenames or equal titles; full identity remains accessible when truncated.
    - Selection, unread, waiting, running, stopped/interrupted and error remain distinct; color is supplementary, not the sole indicator. Model/mode/branch information is shown only from the session's actual data.
    - Switching preserves draft, attachments, composer mode, transcript scroll and pending overlay state; replies reach the originating request/session even when selection changes during delivery.
    - Selecting a tab does not replace its model, alter assignment priority or acknowledge unread content that was not actually viewed. Existing interrupted-child assignment semantics are preserved and documented rather than silently redesigned.
verification_plan:
    - Use two temporary projects with equal basenames/titles and distinct model/mode/branch values; assert selector/composer identity and truncation behavior at narrow and wide widths without changing theme ownership.
    - Switch with drafts, attachments, scroll and each ask kind; deliver a late event and reply while switching, asserting only the originating request is resolved.
    - Cover unread-at-bottom behavior and interrupted-child compatibility using existing public retained-session tests; do not equate tab activation with viewed content.
    - Run scoped render/dispatch tests and race checks only for changed packages; one scoped lint at most. Update actual UI documentation with the implementation.
created_at: "2026-09-04T07:31:55.430542Z"
updated_at: "2026-09-06T14:03:16.862217Z"
---

## Body

**What to deliver.** Make the cross-project tab switch unmistakable and safe through the existing selector/composer/overlay path. This is hardening of the project label supplied by [multisession-projects](multisession-projects.md), not a new sidebar or duplicate header system.

**SOURCE baseline (1a4cf31; relevant code unchanged at 05ce564).** Top selector, retained per-session View state and activation-dependent input already exist. The old statement that only transcript content changes on switching is no longer accurate. The focused selector delivery is recorded in [multisession-background-attention](multisession-background-attention.md); selection dot is not unread.

**Remaining work.** Check active/project identity through same-name directories, long titles, narrow terminals, model/mode updates and background branch events. Consume session-owned values, not the startup project. Preserve full path access through an existing appropriate UI route without requiring a new persistent row. Verify attachments, drafts, scroll and modal replies across switches. No focus theft or cross-session approval; late/dismissed requests must not authorize a successor request. If a request-generation defect is reproduced, fix or explicitly block that dependent criterion rather than treating selection as proof of correct routing.

**Compatibility.** Ordinary running work continues after switching. Existing LeaveAssignment behavior for an already interrupted child is part of the approved child contract, not evidence that ordinary background work should stop. Do not silently remove it. Viewing a tab is not a routing-priority update.

**Deferred styling.** Per-session accent palettes, a mandatory extra one-line header, footer aggregate counts and a toast for every switch were earlier design proposals. They are not needed if the existing cues meet the identity/accessibility criteria; coordinate with active theme work rather than changing its files.

**Blocked by:** [multisession-projects](multisession-projects.md). The completed registry/selector are reused; no grouped-panel prerequisite. Background event delivery remains owned by [multisession-background-attention](multisession-background-attention.md).

**Blocked (2026-09-06).** Waiting for multisession-projects to land the coherent project context and minimal selector label. Reuse that delivery rather than creating a competing label. No sidebar prerequisite; no implementation completion claimed by backlog refresh.

## Acceptance Criteria

- The existing tab selector and active-session cues unambiguously identify session and project, including two projects with identical directory basenames or equal titles; full identity remains accessible when truncated.
- Selection, unread, waiting, running, stopped/interrupted and error remain distinct; color is supplementary, not the sole indicator. Model/mode/branch information is shown only from the session's actual data.
- Switching preserves draft, attachments, composer mode, transcript scroll and pending overlay state; replies reach the originating request/session even when selection changes during delivery.
- Selecting a tab does not replace its model, alter assignment priority or acknowledge unread content that was not actually viewed. Existing interrupted-child assignment semantics are preserved and documented rather than silently redesigned.

## Verification Plan

1. Use two temporary projects with equal basenames/titles and distinct model/mode/branch values; assert selector/composer identity and truncation behavior at narrow and wide widths without changing theme ownership.
2. Switch with drafts, attachments, scroll and each ask kind; deliver a late event and reply while switching, asserting only the originating request is resolved.
3. Cover unread-at-bottom behavior and interrupted-child compatibility using existing public retained-session tests; do not equate tab activation with viewed content.
4. Run scoped render/dispatch tests and race checks only for changed packages; one scoped lint at most. Update actual UI documentation with the implementation.
