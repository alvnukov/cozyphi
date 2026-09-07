---
id: child-screen-esc-leaves
title: Экран ребёнка — Esc выходит в main, Ctrl+C прерывает, второе Ctrl+C не закрывает программу
status: done
priority: high
model_level: high
task_type: bug
parent_id: subagent-panel-ux
tags:
    - agents
    - tui
    - ux
branch: fix/child-screen-esc-leaves
worktree_path: .worktrees/child-screen-esc-leaves
acceptance_criteria:
    - На экране ребёнка простой Esc с пустым ладдером композера (нет пикера, палитры, выделения, очереди на возврат) возвращает в main — и при бегущем, и при завершённом ребёнке. В main Esc ведёт себя как раньше (прерывает ход).
    - Ctrl+C на экране ребёнка прерывает ход ребёнка, как и сейчас; повторное Ctrl+C на экране ребёнка не закрывает cozyphi, а повторяет прерывание (защита от двойного нажатия действует только на экране ребёнка; в main двойное Ctrl+C закрывает как раньше).
    - Ctrl+] (agent-back) убран из таблицы и каталога; путь назад с клавиатуры — Esc, плюс ↓ → Enter на строке main.
    - Футер экрана ребёнка из каталога ScopeChild читается «Esc main · Ctrl+C interrupt» (точный текст через keys.Hints).
    - doc/tui.md, AGENTS_DESIGN.md, internal/tui/DESIGN.md (если упоминает agent-back), CHANGELOG Unreleased — абзац предыдущего фикса про Esc/Ctrl+] переписан под итоговый контракт, часть про курсор ❯ сохранена; тексты на английском.
verification_plan:
    - Тест composer — на экране ребёнка при busy Esc после RecallQueued зовёт seam выхода, а не CancelStreamMsg; в main Esc при busy публикует CancelStreamMsg.
    - Тест на двойное Ctrl+C — на экране ребёнка второе нажатие не даёт quit; в main даёт.
    - Тест keys — хинт ScopeChild равен «Esc main · Ctrl+C interrupt»; таблица без agent-back.
    - Гейты только по изменённым пакетам; один прогон golangci-lint по ним.
created_at: "2026-09-07T10:50:00Z"
updated_at: "2026-09-07T10:45:00Z"
---

## Body

**Откуда.** Обсуждение с пользователем 2026-09-07 после отмены child-screen-esc-keys (Shift+Esc/Ctrl+Esc отпали: без kitty-протокола неотличимы от Esc). Выбран вариант «Esc выходит, Ctrl+C прерывает»: ни одной новой клавиши, работает в любом терминале. Ctrl+C уже прерывает ход в любой сессии (каталог: «interrupt the run; pressed twice in a row, quit»), поэтому на экране ребёнка второе нажатие не должно закрывать программу.

**Что откатывается.** Из subagent-panel-review-followups: Esc-прерывание на экране ребёнка и Ctrl+]. Остаётся: курсор ❯ в панели, фикс парсера xui для 0x1c–0x1f.

**Done (2026-09-07).** Слито в main (279dc90, ветка и воркдри удалены). Esc на экране ребёнка — последняя ступень ладдера композера через сим `SetLeaveOnEscapeFunc` → `family.backFrom`; при бегущем ребёнке вместо CancelStreamMsg, при завершённом — после снятия выделения; в main Esc по-прежнему прерывает ход. `AcceptInterrupt` на экране ребёнка не взводит выход: повторное Ctrl+C прерывает снова, при пустом ходе тост «Nothing running · Esc returns to main». `agent-back`/Ctrl+] удалены из таблицы и каталога. Футер ребёнка: `Esc main · Ctrl+C interrupt`. Доки: doc/tui.md, AGENTS_DESIGN.md, internal/tui/DESIGN.md, CHANGELOG (абзац Fixed переписан, часть про ❯ сохранена). Гейты по composer/sessions/keys + go build ./cmd после слияния зелёные, lint 0. Нужна пересборка бинаря пользователем.
