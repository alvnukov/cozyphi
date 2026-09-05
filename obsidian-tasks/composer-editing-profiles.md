---
id: composer-editing-profiles
title: Selectable standard, Readline and Vim editing with visible modes and undo
status: done
priority: high
task_type: feature
parent_id: cozyphi-convenience-program
branch: feature/composer-editing-profiles
worktree_path: .worktrees/composer-editing-profiles
acceptance_criteria:
    - Standard remains the default; /keymap offers standard/readline/vim and persists choice without losing drafts.
    - Readline editing chords and Vim core motions/operators work with Unicode and multiline drafts; undo/redo recover edits.
    - Global commands remain accessible and configurable; help and composer show active bindings and Vim state; modal/search/voice routing remains intact.
    - Focused tests, formatter, lint and touched-file language diagnostics pass; unrelated theme changes remain untouched.
verification_plan:
    - Test chat editing through public Handle including Unicode, boundaries, undo, mode switching and drawing at narrow widths.
    - Test profile binding conflicts, command dispatch, persistence and picker/escape routing.
    - Run make fmt, fmt-check and lint; focused UI/project package tests.
created_at: "2026-09-05T10:24:13.756018Z"
updated_at: "2026-09-05T10:24:13.756018Z"
---

## Acceptance Criteria

- Standard remains the default; /keymap offers standard/readline/vim and persists choice without losing drafts.
- Readline editing chords and Vim core motions/operators work with Unicode and multiline drafts; undo/redo recover edits.
- Global commands remain accessible and configurable; help and composer show active bindings and Vim state; modal/search/voice routing remains intact.
- Focused tests, formatter, lint and touched-file language diagnostics pass; unrelated theme changes remain untouched.

## Verification Plan

1. Test chat editing through public Handle including Unicode, boundaries, undo, mode switching and drawing at narrow widths.
2. Test profile binding conflicts, command dispatch, persistence and picker/escape routing.
3. Run make fmt, fmt-check and lint; focused UI/project package tests.
