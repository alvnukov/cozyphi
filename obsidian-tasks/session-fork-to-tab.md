---
id: session-fork-to-tab
title: Форк сессии с сообщения в новую вкладку, /fork, кнопка ⑂
status: todo
priority: high
model_level: high
task_type: feature
parent_id: context-window-management
tags:
    - session
    - multisession
    - context
branch: feature/session-fork-to-tab
worktree_path: .worktrees/session-fork-to-tab
acceptance_criteria:
    - session.Fork создаёт новый файл с ParentSession/ForkedFrom и копией пути до якоря с теми же id; исходный файл байт в байт не изменён
    - Форк открывается новой вкладкой того же проекта; исходная вкладка сохраняет транскрипт, лист и черновик
    - Кнопка ⑂ и /fork [<id>] эквивалентны; /fork без аргумента дублирует сессию от текущего листа
    - Отказ во время хода и на промежуточном ряду — с понятной ошибкой
    - Строка в CHANGELOG под Unreleased
verification_plan:
    - go test ./internal/session/... ./internal/tui/sessions/... ./internal/tui/commands/...
    - 'Ручная проверка: форк кнопкой со второго ответа → новая вкладка с историей до него → ход в форке не появляется в исходной; перезапуск и cozyphi sessions list показывает обе'
    - Один scoped golangci-lint run по изменённым пакетам перед коммитом
created_at: "2026-09-16T08:00:30.675171Z"
updated_at: "2026-09-16T08:00:30.675171Z"
---

## Body

**Родитель:** context-window-management (эпик). Зависит от transcript-message-actions и session-leaf-rewind (валидатор границы хода).

**Что.** `session.Fork(src *Manager, anchorID) (*Manager, error)`: новый файл сессии в том же каталоге; заголовок с `ParentSession` = исходная сессия и `ForkedFrom` = якорь; копия `GetBranch(anchor)` с сохранёнными id записей (replay и ссылки aside продолжают работать). Якорь — граница хода по общему валидатору; исходный файл не изменяется.

**UI.** `View.ForkFrom(id)`: форк → новая вкладка через `sessions.Registry`/`View` тем же путём, что `/new` + resume (тот же проект и cwd); исходная вкладка остаётся как была; при форке «до промпта» текст промпта — в композер новой вкладки; тост с именем новой вкладки. Кнопка `⑂` подключается к `ForkFrom`. Команда `/fork [<id>]` — без аргумента форк от текущего листа (дубликат сессии).

**Вне скоупа:** форк в другой проект; слияние веток; заголовки вкладок сверх существующего механизма.

## Acceptance Criteria

- session.Fork создаёт новый файл с ParentSession/ForkedFrom и копией пути до якоря с теми же id; исходный файл байт в байт не изменён
- Форк открывается новой вкладкой того же проекта; исходная вкладка сохраняет транскрипт, лист и черновик
- Кнопка ⑂ и /fork [<id>] эквивалентны; /fork без аргумента дублирует сессию от текущего листа
- Отказ во время хода и на промежуточном ряду — с понятной ошибкой
- Строка в CHANGELOG под Unreleased

## Verification Plan

1. go test ./internal/session/... ./internal/tui/sessions/... ./internal/tui/commands/...
2. Ручная проверка: форк кнопкой со второго ответа → новая вкладка с историей до него → ход в форке не появляется в исходной; перезапуск и cozyphi sessions list показывает обе
3. Один scoped golangci-lint run по изменённым пакетам перед коммитом
