---
id: aside-entry-and-engine-ask
title: 'Ядро /btw: запись aside вне контекста и одноразовый запрос движка'
status: todo
priority: high
model_level: high
task_type: feature
parent_id: context-window-management
tags:
    - session
    - engine
    - aside
    - context
branch: feature/aside-entry-and-engine-ask
worktree_path: .worktrees/aside-entry-and-engine-ask
acceptance_criteria:
    - EntryAside пишется в файл, восстанавливается при загрузке и не появляется в BuildContext ни до, ни после перезапуска
    - Engine.Aside отвечает по контексту якоря, не меняет лист, кэш контекста и историю; во время хода отказывает
    - /btw <вопрос> и /btw @<id> <вопрос> работают; ответ стримится через AsideUpdate
    - Tool-вызовы в ответе aside не исполняются
    - Строка в CHANGELOG под Unreleased
verification_plan:
    - go test ./internal/session/... ./internal/agent/... ./internal/tui/commands/...
    - 'Ручная проверка: два хода → /btw «что мы обсуждали?» → следующий ход не видит вопроса/ответа (проверить через /context) → перезапуск, файл содержит aside'
    - Один scoped golangci-lint run по изменённым пакетам перед коммитом
created_at: "2026-09-16T08:00:30.676392Z"
updated_at: "2026-09-16T08:00:30.676392Z"
---

## Body

**Родитель:** context-window-management (эпик). Зависит от live-transcript-entry-ids.

**Что.** Новая запись `EntryAside {ID, ParentID (лист на момент вопроса), AnchorID, Question, Answer, Model, Tokens}` — как `session_title`: не двигает лист и никогда не входит в `BuildContext`. `Engine.Aside(ctx, question, anchorID)`: контекст = `GetBranch(anchor)` (пустой якорь — текущий путь) + вопрос как user-сообщение; вызов `client.Stream` напрямую по образцу session/compaction, без `Session.Append*`; стриминг в UI через новое событие `session.Event` `AsideUpdate` (id записи стабилен от первого токена); по завершении запись aside в лог. Tool-вызовы модели не исполняются: запрос без инструментов, если клиент это позволяет, иначе остановка на первом tool_use с пометкой в ответе. Отказ во время хода. Ответ aside — обычный текст модели, не untrusted-фрейм; web-taint хода не затрагивается.

**Команда.** `/btw <вопрос>` — по текущему контексту; `/btw @<id> <вопрос>` — по контексту на момент записи id (любая запись сообщения, включительно). Ответ в этой задаче показывается временно как обычная assistant-строка с префиксом «btw:» — красивый блок делает aside-transcript-row.

**Вне скоупа:** блок в ленте, режим композера, выбор модели для aside.

## Acceptance Criteria

- EntryAside пишется в файл, восстанавливается при загрузке и не появляется в BuildContext ни до, ни после перезапуска
- Engine.Aside отвечает по контексту якоря, не меняет лист, кэш контекста и историю; во время хода отказывает
- /btw <вопрос> и /btw @<id> <вопрос> работают; ответ стримится через AsideUpdate
- Tool-вызовы в ответе aside не исполняются
- Строка в CHANGELOG под Unreleased

## Verification Plan

1. go test ./internal/session/... ./internal/agent/... ./internal/tui/commands/...
2. Ручная проверка: два хода → /btw «что мы обсуждали?» → следующий ход не видит вопроса/ответа (проверить через /context) → перезапуск, файл содержит aside
3. Один scoped golangci-lint run по изменённым пакетам перед коммитом
