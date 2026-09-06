---
id: multisession-background-attention
title: Finish origin-bound background attention for session tabs
status: todo
priority: high
model_level: high
task_type: feature
parent_id: multisession-mode
tags:
    - cozyphi
    - tui
    - notify
    - multisession
branch: feature/multisession-background-attention
worktree_path: .worktrees/multisession-background-attention
acceptance_criteria:
    - Background permission/question/continue/completion/error events update the originating tab's attention state and provide a discoverable way to select it without stealing active input.
    - Attention actions retain live session identity across tab reorder/close; selecting a notice only navigates and never submits an answer or grants permission.
    - Session-labelled desktop notifications obey configured off/always/unfocused policy. Completion unread clears only after the selected transcript is actually viewed at bottom, not on selection alone.
    - Existing running/waiting/interrupted/stopped/error/unread/live-job distinctions remain correct. Voice/input ownership on switching is verified through controlled adapters; device behavior is not claimed without a device check.
    - Focused selector/notice behavior already delivered remains regression-covered; missing keyboard access to attention is addressed using existing navigation or an explicit focus action, never a global Enter handler.
verification_plan:
    - With controlled sessions, deliver each background ask, completion and error while typing in another tab; assert origin labels, status, no focus theft and no automatic response.
    - Activate notices by supported mouse/keyboard routes, close or reorder the target before activation, and verify stale targets never redirect to a surviving tab.
    - Test unread with selected/not-drawn/scrolled-up/modal-obscured views; test notifier policy and voice ownership through adapters. Record any device smoke separately.
    - Run format/build/tests/race only in packages changed by this task and at most one scoped lint before commit; no live provider, microphone or desktop notification calls without separate approval.
created_at: "2026-09-04T07:31:55.432059Z"
updated_at: "2026-09-06T14:00:47.462015Z"
---

## Body

**What to deliver.** Complete origin-bound attention for tabbed sessions without a grouped sidebar or input theft. The already delivered selector/notice is the implementation base, not something to build again.

**SOURCE baseline.** The main/child selector already has status and job marks, origin-labelled clickable attention, and session-labelled desktop notifications. Historical merge/test/smoke evidence below is preserved verbatim; it was not rerun during the 2026-09-06 backlog reconciliation. Investigation at 1a4cf31 found per-View buses and activation/attention tracking; relevant source is unchanged at 05ce564.

**Remaining work.** Verify every ask/completion/error path and stale notice target through the existing shell, fill actual origin/status gaps, and make the attention destination discoverable from keyboard navigation. If Enter activation is added, it must require explicit notice focus and must not capture ordinary composer Enter. Coalesce repeated attention for one session without hiding errors or approving work. Test configured desktop notification policies through an adapter, not live OS notifications. Preserve unread's actual-view condition, job ownership and distinct selection/status marks. Verify voice callbacks cannot submit into another tab; do not claim microphone or OS-focus behavior from fake tests.

**Coordination.** [multisession-hotkeys](multisession-hotkeys.md) owns global navigation/help; consume resolved hints rather than hardcoding Alt+2. [multisession-switch-cues](multisession-switch-cues.md) owns switching identity/overlay response tests. OS click-to-focus behavior is separately tracked by iterm-notification-focus and is not taken over here. Grouped-sidebar integration is deferred to [multisession-sessions-panel](multisession-sessions-panel.md), not a completion gate.

**Blocked by:** [multisession-registry](multisession-registry.md) (done). No sessions-panel prerequisite.

**Historical delivery record (the following original notes are evidence, not the current remaining scope).**

**Started (2026-09-06).** Implement approved focused child selector/attention contract in own worktree, reusing retained Registry/View/App/keys and notifier. Broader grouped multi-project sidebar requirements will not be claimed complete. Atomic Engine selection proceeds independently on its own branch. No lint rerun.

**Note (2026-09-06).** Focused selector slice committed6c0ddbe in own worktree; integrated with model branch (merge pending final checks). Cancelled agent patch inspected and finished directly: unread counts completed turns only, stop-before-stream uses Assignment snapshot, selector/composer origin and clickable attention, existing keys/no focus theft, per-notifier origin titles. w30 editor/notify passed; sessions failed only double unread; fixed with w31 regression green. Broader grouped sidebar/focusable-toast task criteria are not claimed complete. No agents, lint rerun or push.

**Reopened (2026-09-06).** Focused interactive-child selector/attention slice landed main75ade1c via6c0ddbe/e69fe05: clickable main/child identity, distinct running/waiting/interrupted/stopped/error/unread/live-job marks; origin-labelled clickable attention notice and desktop notification titles; completed-turn unread cleared only when selected transcript viewed at bottom. W33 affected-package race tests and build passed; terminal smoke verified background completion without focus theft, unread marker, click notice selects retained child and clears unread. Broader task stays TODO: grouped sessions-panel integration and keyboard-focusable toast/Enter behavior are not implemented by this slice; no microphone/device notification smoke claimed. No repeat lint or push. The completed focused worktree can be cleaned; future continuation should prepare a fresh worktree from main.

## Acceptance Criteria

- Background permission/question/continue/completion/error events update the originating tab's attention state and provide a discoverable way to select it without stealing active input.
- Attention actions retain live session identity across tab reorder/close; selecting a notice only navigates and never submits an answer or grants permission.
- Session-labelled desktop notifications obey configured off/always/unfocused policy. Completion unread clears only after the selected transcript is actually viewed at bottom, not on selection alone.
- Existing running/waiting/interrupted/stopped/error/unread/live-job distinctions remain correct. Voice/input ownership on switching is verified through controlled adapters; device behavior is not claimed without a device check.
- Focused selector/notice behavior already delivered remains regression-covered; missing keyboard access to attention is addressed using existing navigation or an explicit focus action, never a global Enter handler.

## Verification Plan

1. With controlled sessions, deliver each background ask, completion and error while typing in another tab; assert origin labels, status, no focus theft and no automatic response.
2. Activate notices by supported mouse/keyboard routes, close or reorder the target before activation, and verify stale targets never redirect to a surviving tab.
3. Test unread with selected/not-drawn/scrolled-up/modal-obscured views; test notifier policy and voice ownership through adapters. Record any device smoke separately.
4. Run format/build/tests/race only in packages changed by this task and at most one scoped lint before commit; no live provider, microphone or desktop notification calls without separate approval.
