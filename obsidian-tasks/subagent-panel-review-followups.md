---
id: subagent-panel-review-followups
title: Панель сабагентов после живой проверки — Esc снова прерывает, выход отдельной клавишей, выделение строки как у Claude Code
status: in_progress
priority: high
model_level: high
task_type: bug
parent_id: subagent-panel-ux
tags:
    - agents
    - tui
    - ux
branch: fix/subagent-panel-review-followups
worktree_path: .worktrees/subagent-panel-review-followups
acceptance_criteria:
    - Esc в композере ребёнка ведёт себя как в main — при бегущем ходе прерывает его (после возврата очереди), при пустом ладдере ничего не делает; в main никаких изменений. Шов SetLeaveEscapeFunc/escapeFrom удалён или больше не привязан к Esc.
    - Возврат на экран родителя — отдельная клавиша из каталога keys (Ctrl+], если xui её различает; иначе Ctrl+G), работает и при бегущем, и при завершённом ребёнке, только когда открыт экран ребёнка; ↓ → Enter на строке main по-прежнему работает. Футер экрана ребёнка называет новую клавишу вместо «Esc main».
    - Выбранная строка панели не заливается reverse на всю ширину — слева ставится курсор `❯ ` (у прочих строк два пробела), стили головы и хвоста прежние; `● main` жирным. Тесты панели читают выбор по курсору, а не по Reverse.
    - doc/tui.md, AGENTS_DESIGN.md (места про Esc на экране ребёнка), CHANGELOG Unreleased обновлены; хинты только из каталога keys.
verification_plan:
    - Тест composer — Esc при busy на экране ребёнка публикует CancelStreamMsg, а не уходит; тест view/family — новая клавиша зовёт open("") только для текущего ребёнка.
    - Тест agentpanel — выбранная строка начинается с ❯, ни одна ячейка не Reverse.
    - Гейты только по изменённым пакетам (composer, sessions, agentpanel, keys); один прогон golangci-lint по ним.
created_at: "2026-09-07T09:50:00Z"
updated_at: "2026-09-07T09:50:00Z"
---

## Body

**Откуда.** Живая проверка пользователем 2026-09-07 после слияния subagent-child-screen-exit и subagent-panel-live-action: «выход по Esc … тупо же, теперь я прервать агента не могу, выход надо делать другой» и «внизу переключение агентов не изменилось, не так аккуратно как у Клода — строка полностью подсвечивается».

**Причина.** В subagent-child-screen-exit я сам сделал Esc в композере ребёнка выходом в main (composer.SetLeaveEscapeFunc → family.escapeFrom): при бегущем ходе рунг стоит раньше CancelStreamMsg, поэтому прервать ребёнка с клавиатуры стало нельзя. В agentpanel.drawRow выбранная строка красится xui.Style{Reverse:true} на всю ширину; на скриншотах Claude Code выбор помечен только `❯` слева, без заливки.

**Решение.** Esc возвращает себе обычный смысл на любом экране. Выход с экрана ребёнка — отдельная клавиша из каталога (ScopeChild), с проверкой, что xui её парсит. Курсор `❯` вместо reverse.
