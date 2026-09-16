---
id: docs-context-window-management
title: 'Документация: дерево сессии, откат, форк, aside, кнопки в ленте'
status: todo
priority: medium
model_level: low
task_type: docs
parent_id: context-window-management
tags:
    - docs
    - session
    - tui
branch: docs/docs-context-window-management
worktree_path: .worktrees/docs-context-window-management
acceptance_criteria:
    - doc/session.md, doc/tui.md, AGENTS.md, AGENTS_DESIGN.md и help описывают реализованное поведение без расхождений с кодом
    - Ссылки между документами валидны
verification_plan:
    - 'Markdown-only: проверка ссылок и diff; Go-гейты не запускаются'
    - Сверка каждого утверждения с кодом соответствующей дочерней задачи
created_at: "2026-09-16T08:00:30.680216Z"
updated_at: "2026-09-16T08:00:30.680216Z"
---

## Body

**Родитель:** context-window-management (эпик). Зависит от всех остальных детей, кроме context-pane-message-actions.

**Что.** `doc/session.md`: дерево записей, лист, записи leaf и aside, откат/форк/undo, что входит и не входит в контекст. `doc/tui.md`: полоса кнопок в сообщениях, потоки rewind/fork/aside через `commands.Host`, режим btw композера, новый токен темы. `AGENTS.md`: инвариант «лог сессии — дерево: откат двигает лист, форк копирует путь, aside никогда не входит в контекст». Таблица клавиш в help (`internal/tui/keys/keys.go`) — Ctrl+T и кнопки. `AGENTS_DESIGN.md`: fork больше не «не делаем», ссылка на эпик. Проверить, что строки CHANGELOG детей согласованы.

**Вне скоупа:** код.

## Acceptance Criteria

- doc/session.md, doc/tui.md, AGENTS.md, AGENTS_DESIGN.md и help описывают реализованное поведение без расхождений с кодом
- Ссылки между документами валидны

## Verification Plan

1. Markdown-only: проверка ссылок и diff; Go-гейты не запускаются
2. Сверка каждого утверждения с кодом соответствующей дочерней задачи
