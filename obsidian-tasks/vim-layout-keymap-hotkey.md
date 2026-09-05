---
id: vim-layout-keymap-hotkey
title: Support alternate layouts in Vim and F6 keymap cycling
status: done
priority: medium
model_level: medium
task_type: bug
branch: bug/vim-layout-keymap-hotkey
worktree_path: .worktrees/vim-layout-keymap-hotkey
acceptance_criteria:
    - Vim NORMAL uses shared alternate-layout normalization; INSERT preserves text.
    - Rebindable F6 cycles keymaps, preserves draft/caret, persists choice and appears in help.
verification_plan:
    - Reproduce layout issue with focused regression tests.
    - Run scoped formatting, tests and lint.
    - Commit and merge; close task and clean worktree.
created_at: "2026-09-05T18:50:43.647403Z"
updated_at: "2026-09-05T19:14:21.312983Z"
---

## Body

Fix alternate keyboard layout commands in Vim NORMAL using shared hotkey normalization. Add F6 keymap cycling through standard/readline/vim using existing profile switching, with Vim starting in INSERT. Update help, tests and changelog.

**Done (2026-09-05).** Implemented shared NORMAL layout normalization and configurable F6 keymap cycling, preserving draft/caret and saved preference; Readline verbose is Shift+F6. Tests and help/docs updated. Code commit 95db7ed merged into main. Scoped chat/keys/sessions tests pass; sessions retested after integration. One scoped lint found test nesting warning, corrected and keys retested; lint not rerun per requested limit. No push.

## Acceptance Criteria

- Vim NORMAL uses shared alternate-layout normalization; INSERT preserves text.
- Rebindable F6 cycles keymaps, preserves draft/caret, persists choice and appears in help.

## Verification Plan

1. Reproduce layout issue with focused regression tests.
2. Run scoped formatting, tests and lint.
3. Commit and merge; close task and clean worktree.
