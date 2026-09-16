---
id: session-leaf-rewind
title: 'Откат контекста: сдвиг листа дерева сессии, /rewind, кнопка ↶'
status: todo
priority: high
model_level: high
task_type: feature
parent_id: context-window-management
tags:
    - session
    - engine
    - context
branch: feature/session-leaf-rewind
worktree_path: .worktrees/session-leaf-rewind
acceptance_criteria:
    - Manager.Rewind добавляет запись leaf, переставляет курсор, BuildContext возвращает путь до якоря; загрузка файла восстанавливает курсор; UndoRewind возвращает предыдущий лист
    - Откат допускает только границы ходов; на промежуточный ряд — отказ с понятной ошибкой
    - Engine.Rewind отказывает во время хода; после отката следующий ход продолжается от нового листа и пишется как новая ветка
    - Транскрипт после отката показывает только путь; текст промпта-якоря возвращён в композер; /rewind back восстанавливает прежний вид
    - Кнопка ↶ и /rewind <id>/back делают одно и то же; ArgCompleter показывает границы ходов
    - Строка в CHANGELOG под Unreleased
verification_plan:
    - go test ./internal/session/... ./internal/agent/... ./internal/tui/sessions/... ./internal/tui/commands/...
    - 'Ручная проверка: три хода → откат кнопкой к первому промпту → новый ход → /rewind back; перезапуск и /resume показывает тот же путь'
    - Один scoped golangci-lint run по изменённым пакетам перед коммитом
created_at: "2026-09-16T08:00:30.674149Z"
updated_at: "2026-09-16T08:00:30.674149Z"
---

## Body

**Родитель:** context-window-management (эпик). Зависит от live-transcript-entry-ids и transcript-message-actions.

**Что.** Новая запись сессии `EntryLeaf {ID, ParentID, Target, From}` — перемещение курсора: `Manager.Rewind(entryID)` проверяет границу хода, добавляет запись и переставляет `leafID` на Target; при загрузке файла запись восстанавливает курсор; `Manager.UndoRewind()` возвращает лист на From. Старая ветка остаётся в файле (append-only). Общий валидатор границы хода (промпт — «до него», последний assistant без незакрытых tool-вызовов — «после него») живёт в `session` и переиспользуется форком.

**Движок.** `agent.Session` сбрасывает кэш контекста; `Engine.Rewind(id)` под локом, отказ во время хода (как CompactNow). Если ветка после отката не помещается в окно — работает существующий `ErrCompactionRequired`.

**UI.** `View.RewindTo(id)`: движок → транскрипт перестраивается `LoadReplay(ReplaySnapshot(PathEntries()))` (строки после листа исчезают) → при откате «до промпта» текст промпта возвращается в композер → тост «Откат до …; /rewind back — вернуть». Кнопка `↶` из transcript-message-actions подключается к `RewindTo`. Команда `/rewind [<id>|back]` с `ArgCompleter`, перечисляющим границы ходов (id + превью).

**Вне скоупа:** переключение между ветками в UI; форк; aside.

## Acceptance Criteria

- Manager.Rewind добавляет запись leaf, переставляет курсор, BuildContext возвращает путь до якоря; загрузка файла восстанавливает курсор; UndoRewind возвращает предыдущий лист
- Откат допускает только границы ходов; на промежуточный ряд — отказ с понятной ошибкой
- Engine.Rewind отказывает во время хода; после отката следующий ход продолжается от нового листа и пишется как новая ветка
- Транскрипт после отката показывает только путь; текст промпта-якоря возвращён в композер; /rewind back восстанавливает прежний вид
- Кнопка ↶ и /rewind <id>/back делают одно и то же; ArgCompleter показывает границы ходов
- Строка в CHANGELOG под Unreleased

## Verification Plan

1. go test ./internal/session/... ./internal/agent/... ./internal/tui/sessions/... ./internal/tui/commands/...
2. Ручная проверка: три хода → откат кнопкой к первому промпту → новый ход → /rewind back; перезапуск и /resume показывает тот же путь
3. Один scoped golangci-lint run по изменённым пакетам перед коммитом
