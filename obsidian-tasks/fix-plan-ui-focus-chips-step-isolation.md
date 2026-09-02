---
id: fix-plan-ui-focus-chips-step-isolation
title: 'Plan UI: редактор правил все шаги разом, чипы без скиллов, planFocus ел клавиши композера'
status: done
task_type: bug
tags:
    - tui
    - plan
    - focus
    - sidebar
    - planedit
created_at: "2026-08-29T17:40:08.83394Z"
updated_at: "2026-08-29T17:40:08.83394Z"
---

## Body

Found while testing the plan tool UI in the harness (2026-08-29); all four fixed on main in the same session (plan revision 61, `make fmt lint test` green).

1. Plan editor edited all steps at once instead of the selected one — ambiguous identity in browse/detail/popup titles; editing now names the step everywhere and compiles to exactly one update_step op (internal/tui/planedit).
2. Sidebar action chip showed only "inject_skill@step_start" — skills never rendered; chips now list them (internal/tui/sidebar actionChipText).
3. Model picker seemed missing — it existed but planFocus leaked: a step click or alt+P left planFocus=true while ChatInput held real focus, so HandlePlanKey ate ↑↓/Enter/Esc/m before the composer. editor.Handle now calls Sidebar.ReleasePlanFocus when real focus is off the editor root; the picker is reachable via alt+P → m.
4. Control keys swallowed while typing — same root cause, same fix.

Kept as a record of the bug class: keyboard-mode flags must be released when real focus moves, not only on their own Esc.
