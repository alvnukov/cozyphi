---
id: slash-command-history
title: 'Slash command history: record picker-accepted commands and slash-only Up/Down recall'
status: done
priority: medium
task_type: feature
parent_id: cozyphi-convenience-program
tags:
    - tui
    - composer
branch: feature/slash-command-history
worktree_path: .worktrees/slash-command-history
acceptance_criteria:
    - Slash command executed via the / picker (no args) appears in prompt history and survives restart
    - Up in an empty composer recalls the newest submission (mixed, incl. slash commands)
    - Up/Down from a '/'-leading draft with the picker closed walks only slash entries; Down restores the draft
    - Picker Up/Down navigation unchanged while open
    - make fmt-check lint test green
verification_plan:
    - go test ./internal/history/... ./internal/components/chat/... ./internal/tui/composer/...
    - make fmt-check lint test in the task worktree
    - 'manual: phi, /clear via picker, Esc, Up — /clear recalled'
created_at: "2026-09-03T16:07:12.495884Z"
updated_at: "2026-09-03T17:03:59.680117Z"
---

## Body

**Problem:** slash commands have no usable history. Commands accepted from the
`/` picker (most no-arg commands) publish `SubmitMsg` directly from
`ComposerPane.acceptSlash`, bypassing `Chat.OnSubmit` and therefore
`history.Append` — they never reach `~/.cozyphi/prompt-history.jsonl`. And the
history walk has no slash notion: recall from a `/` draft would surface plain
prompts.

**Fix:** route picker-accepted no-arg commands through `Chat.OnSubmit` (one
submit path, recording included); extend `history.Store` so a walk started from
a `/`-leading draft visits only slash entries (filter fixed at walk start,
draft capture and divergence refusal unchanged).

**User decisions (2026-09-04):** history = Up/Down recall of entered slash
commands with arguments (not palette reordering); persistent across sessions
(existing `prompt-history.jsonl`).

**Surfaces:** internal/history/history.go + tests; internal/tui/composer/pane.go
(acceptSlash) + tests; keys hint (internal/tui/keys); CHANGELOG.

**Done (2026-09-03).** Landed on main: ca290b2 (feature commit, worktree) merged as 034342c. history.Store walk now visits only slash entries when started from a '/'-leading draft (draft capture and divergence refusal unchanged); ComposerPane.acceptSlash routes picker-accepted no-arg commands through Chat.OnSubmit so they are recorded in prompt-history.jsonl like typed commands; keys hint and doc/tui.md updated. Gates make fmt-check lint test green in the worktree; focused packages re-tested green on main after merge. No push (not requested).

## Acceptance Criteria

- Slash command executed via the / picker (no args) appears in prompt history and survives restart
- Up in an empty composer recalls the newest submission (mixed, incl. slash commands)
- Up/Down from a '/'-leading draft with the picker closed walks only slash entries; Down restores the draft
- Picker Up/Down navigation unchanged while open
- make fmt-check lint test green

## Verification Plan

1. go test ./internal/history/... ./internal/components/chat/... ./internal/tui/composer/...
2. make fmt-check lint test in the task worktree
3. manual: phi, /clear via picker, Esc, Up — /clear recalled
