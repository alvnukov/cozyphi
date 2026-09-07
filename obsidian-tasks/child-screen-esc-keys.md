---
id: child-screen-esc-keys
title: Экран ребёнка — Esc выходит, Shift+Esc/Ctrl+Esc прерывают, подсказка называет обе
status: in_progress
priority: high
model_level: high
task_type: bug
parent_id: subagent-panel-ux
tags:
    - agents
    - tui
    - ux
branch: fix/child-screen-esc-keys
worktree_path: .worktrees/child-screen-esc-keys
acceptance_criteria:
    - На экране ребёнка простой Esc с пустым ладдером композера (нет пикера, палитры, выделения, очереди на возврат) возвращает в main — и при бегущем, и при завершённом ребёнке. В main Esc ведёт себя как раньше (прерывает ход).
    - Shift+Esc и Ctrl+Esc — команда прерывания хода из каталога keys; на экране ребёнка прерывают ребёнка, в main прерывают main (тот же CancelStreamMsg, что и Esc в main). Ctrl+C не трогать.
    - Ctrl+] (agent-back) убран из таблицы и каталога; Esc — единственный путь назад с клавиатуры, помимо ↓ → Enter на строке main.
    - Футер экрана ребёнка из каталога ScopeChild называет обе клавиши — «Esc main · Shift+Esc interrupt» (точный текст через keys.Hints); строка подсказки над панелью не меняется.
    - Если терминал не различает Shift+Esc/Ctrl+Esc (без kitty keyboard protocol / modifyOtherKeys они приходят как обычный Esc), это честно записано в doc/tui.md с указанием, что тогда прерывать ребёнка нужно Ctrl+C или x в панели; проверить, включает ли xui kitty-протокол при старте, и написать как есть.
    - doc/tui.md, AGENTS_DESIGN.md, CHANGELOG Unreleased обновлены; тексты на английском.
verification_plan:
    - Тест composer — на экране ребёнка при busy Esc после RecallQueued зовёт seam выхода, а не CancelStreamMsg; Shift+Esc и Ctrl+Esc публикуют CancelStreamMsg; в main Esc при busy публикует CancelStreamMsg.
    - Тест keys — хинт ScopeChild содержит обе клавиши; таблица без agent-back.
    - Тест xui/input — CSI 27;2u и 27;5u (и modifyOtherKeys 27;2;27~) дают KeyEscape с ModShift/ModCtrl.
    - Гейты только по изменённым пакетам; один прогон golangci-lint по ним.
created_at: "2026-09-07T10:30:00Z"
updated_at: "2026-09-07T10:30:00Z"
---

## Body

**Откуда.** Пользователь 2026-09-07 после subagent-panel-review-followups: «давай shift+Esc и ctrl+Esc на прерывание ребёнка, а просто Esc — выход из ребёнка, но надо подсказку писать про эти клавиши». Это отменяет решение предыдущего тикета (Esc прерывает, Ctrl+] выход).

**Риск.** Модификаторы у Esc различимы только под kitty keyboard protocol или xterm modifyOtherKeys; в остальных терминалах Shift+Esc приходит как Esc и уйдёт в main вместо прерывания. Остаются Ctrl+C и x в панели; записать в doc/tui.md.
