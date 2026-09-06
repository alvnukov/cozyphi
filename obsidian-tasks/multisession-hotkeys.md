---
id: multisession-hotkeys
title: Reconcile tab navigation, key bindings and command help
status: todo
priority: high
model_level: high
task_type: feature
parent_id: multisession-mode
tags:
    - cozyphi
    - tui
    - keys
    - multisession
branch: feature/multisession-hotkeys
worktree_path: .worktrees/multisession-hotkeys
acceptance_criteria:
    - Existing next/previous/back navigation remains available through configured keys; defaults are not silently replaced by the old Alt-key proposal. Existing /switch N, /new, /close and /rename routes remain compatible.
    - Help, palette and any displayed navigation hints agree with the resolved key table, including overrides and conflict validation; they do not advertise absent sidebar or restore actions.
    - Navigation preserves the retained View, draft, overlays and ordinary running work; invalid targets and closing tabs are handled without acting on the wrong live session.
    - Targeted dispatch/override/legacy-terminal tests document supported behavior. Searchable all-project history and new shortcut families are not required to close this V1 task.
verification_plan:
    - Read the current key table and command registrations before editing; exercise next/previous/back, /switch N, overrides and rejected bindings through public dispatch tests.
    - Use retained fake sessions with drafts and pending asks; navigate during background delivery and while one tab closes, checking target identity and input preservation.
    - Run format/build/tests only for changed key/command/editor packages, plus race checks where changed concurrency warrants them; at most one scoped lint before commit.
    - Document only verified bindings. Optional manual terminal checks use a fake/local fixture, no provider request; do not claim iTerm2/tmux/kitty checks that were not run.
created_at: "2026-09-04T07:31:55.428526Z"
updated_at: "2026-09-06T14:00:03.927187Z"
---

## Body

**What to deliver.** Finish the end-to-end navigation contract for existing tabs: a user can discover, invoke and rebind supported navigation without misleading help or unintended loss of state. Do not reimplement the retained registry.

**SOURCE baseline (1a4cf31; relevant code unchanged at 05ce564).** Next/previous/back already use Ctrl+F10, Shift+F10 and Alt+F10 in the key table. /switch N, /new and /close are registered by TUI assembly; switching selects a retained View. Preserve /rename and pinned titles. The former plan to remove /switch and replace defaults with Alt+Up/Down, Alt+1..9 and Alt+backtick is superseded, not an implementation instruction.

**Remaining work.** Audit help/palette/selector hints against resolved key bindings, close only observed gaps, and add route-level regressions for overrides, unknown/conflicting bindings, invalid targets and closing tabs. Dynamic tab order must not turn a captured live target into another session. Navigation should work while an agent runs without sending the navigation text as a prompt. Preserve modal and composer behavior and existing legacy/kitty input support; test claimed terminal behavior rather than assuming every chord is delivered.

**Deferred proposal.** A searchable session list for many tabs remains useful, but the old all-project open-and-recent picker, new Alt-digit family, sidebar toggle/focus commands and new-session shortcut require their own selected scope. They are not mandatory for this task or prerequisites for /new <path>. Do not expose commands for a sidebar that V1 does not require.

**Blocked by:** [multisession-registry](multisession-registry.md) (done). Project creation belongs to [multisession-projects](multisession-projects.md); visual identity to [multisession-switch-cues](multisession-switch-cues.md). Neither a grouped panel nor a complete history picker gates this task.

## Acceptance Criteria

- Existing next/previous/back navigation remains available through configured keys; defaults are not silently replaced by the old Alt-key proposal. Existing /switch N, /new, /close and /rename routes remain compatible.
- Help, palette and any displayed navigation hints agree with the resolved key table, including overrides and conflict validation; they do not advertise absent sidebar or restore actions.
- Navigation preserves the retained View, draft, overlays and ordinary running work; invalid targets and closing tabs are handled without acting on the wrong live session.
- Targeted dispatch/override/legacy-terminal tests document supported behavior. Searchable all-project history and new shortcut families are not required to close this V1 task.

## Verification Plan

1. Read the current key table and command registrations before editing; exercise next/previous/back, /switch N, overrides and rejected bindings through public dispatch tests.
2. Use retained fake sessions with drafts and pending asks; navigate during background delivery and while one tab closes, checking target identity and input preservation.
3. Run format/build/tests only for changed key/command/editor packages, plus race checks where changed concurrency warrants them; at most one scoped lint before commit.
4. Document only verified bindings. Optional manual terminal checks use a fake/local fixture, no provider request; do not claim iTerm2/tmux/kitty checks that were not run.
