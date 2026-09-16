---
id: live-transcript-entry-ids
title: Живые строки транскрипта несут персистентные id записей сессии
status: todo
priority: high
model_level: medium
task_type: bug
parent_id: context-window-management
tags:
    - session
    - transcript
    - context
branch: bug/live-transcript-entry-ids
worktree_path: .worktrees/live-transcript-entry-ids
acceptance_criteria:
    - После живого хода id строк транскрипта (user и assistant) равны id соответствующих записей в файле сессии
    - Снимок транскрипта после живого хода и снимок после replay того же файла совпадают по id строк (тест)
    - Tail-патч Mapper по-прежнему срабатывает на стриме (id assistant-строки стабилен от первого токена до конца хода)
    - Строка в CHANGELOG под Unreleased
verification_plan:
    - 'go build ./... и go test по изменённым пакетам: ./internal/agent/... ./internal/session/... ./internal/tui/submit/... ./internal/tui/controller/... ./internal/tui/transcript/...'
    - 'Ручная проверка: ход в TUI, затем /context — id записей совпадают с id строк (временный лог или тест на Project)'
    - Один scoped golangci-lint run по изменённым пакетам перед коммитом
created_at: "2026-09-16T08:00:30.672449Z"
updated_at: "2026-09-16T08:00:30.672449Z"
---

## Body

**Родитель:** context-window-management (эпик). Первая задача эпика — на ней держатся кнопки в ленте.

**Дефект.** Живые строки транскрипта не знают персистентных id записей сессии: assistant-строка получает `assistant-<UnixNano>` (internal/agent/engine_stream.go:23) и никогда не заменяется id, который минтит `sess.AppendAssistant`; user-строка получает `session.NewUserMessageID()` в UI (internal/tui/submit/submitter.go, internal/tui/controller/assignment.go), но в `Manager.appendMessage` этот id не передаётся — менеджер минтит свой. Совпадение появляется только после `/resume` (replay берёт `entry.GetID()`). Кнопка «откат/форк/btw» на живой строке обязана знать id записи.

**Решение.** Один режим id: `Session.Append*` возвращает id записи (или принимает заранее выданный id от вызывающего), и тот же id идёт в события `UserAppend`/`AssistantMessageUpdate`/`UserPromoted`; `session.Item.ID` живой строки равен `EntryID`. Замена `assistant-<nanos>` на id, полученный при старте хода из менеджера (или выданный менеджером заранее через `NextID`), — выбрать вариант с наименьшим числом мест изменения и без гонки между стримом и записью.

**Вне скоупа:** сами кнопки, откат, форк, aside.

## Acceptance Criteria

- После живого хода id строк транскрипта (user и assistant) равны id соответствующих записей в файле сессии
- Снимок транскрипта после живого хода и снимок после replay того же файла совпадают по id строк (тест)
- Tail-патч Mapper по-прежнему срабатывает на стриме (id assistant-строки стабилен от первого токена до конца хода)
- Строка в CHANGELOG под Unreleased

## Verification Plan

1. go build ./... и go test по изменённым пакетам: ./internal/agent/... ./internal/session/... ./internal/tui/submit/... ./internal/tui/controller/... ./internal/tui/transcript/...
2. Ручная проверка: ход в TUI, затем /context — id записей совпадают с id строк (временный лог или тест на Project)
3. Один scoped golangci-lint run по изменённым пакетам перед коммитом
