---
id: context-pane-message-actions
title: Паритет откат/форк/btw в браузере /context
status: todo
priority: low
model_level: medium
task_type: feature
parent_id: context-window-management
tags:
    - tui
    - ctxpane
    - context
branch: feature/context-pane-message-actions
worktree_path: .worktrees/context-pane-message-actions
acceptance_criteria:
    - Клавиши r/f/b и пункты меню делают то же, что кнопки в ленте, через те же методы View
    - Недоступные действия (не граница хода, идёт ход) объяснены в футере
    - Строка в CHANGELOG под Unreleased
verification_plan:
    - go test ./internal/tui/ctxpane/... ./internal/tui/sessions/...
    - 'Ручная проверка: /context → r на промпте, f на ответе, b на любой строке'
    - Один scoped golangci-lint run по изменённым пакетам перед коммитом
created_at: "2026-09-16T08:00:30.679049Z"
updated_at: "2026-09-16T08:00:30.679049Z"
---

## Body

**Родитель:** context-window-management (эпик). Зависит от session-leaf-rewind, session-fork-to-tab, aside-entry-and-engine-ask.

**Что.** В `internal/tui/ctxpane`: пункты меню и клавиши `r` (откат: на user-строке «до этого промпта», на assistant-строке «после этого ответа»; недоступно вне границы хода), `f` (форк), `b` (закрывает браузер и включает режим btw композера с якорем). Колбэки `onRewind/onFork/onAside` по образцу `onTrim`. Подсказки в футере браузера отличают trim («голова уходит, хвост остаётся») от rewind («хвост уходит»).

**Вне скоупа:** новые действия за пределами трёх; переработка браузера.

## Acceptance Criteria

- Клавиши r/f/b и пункты меню делают то же, что кнопки в ленте, через те же методы View
- Недоступные действия (не граница хода, идёт ход) объяснены в футере
- Строка в CHANGELOG под Unreleased

## Verification Plan

1. go test ./internal/tui/ctxpane/... ./internal/tui/sessions/...
2. Ручная проверка: /context → r на промпте, f на ответе, b на любой строке
3. Один scoped golangci-lint run по изменённым пакетам перед коммитом
