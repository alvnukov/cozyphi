---
id: live-transcript-entry-ids
title: Живые строки транскрипта несут персистентные id записей сессии
status: done
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
updated_at: "2026-09-20T19:09:21.000000Z"
---

## Body

**Родитель:** context-window-management (эпик). Первая задача эпика — на ней держатся кнопки в ленте.

**Дефект.** Живые строки транскрипта не знают персистентных id записей сессии: assistant-строка получает `assistant-<UnixNano>` (internal/agent/engine_stream.go:23) и никогда не заменяется id, который минтит `sess.AppendAssistant`; user-строка получает `session.NewUserMessageID()` в UI (internal/tui/submit/submitter.go, internal/tui/controller/assignment.go), но в `Manager.appendMessage` этот id не передаётся — менеджер минтит свой. Совпадение появляется только после `/resume` (replay берёт `entry.GetID()`). Кнопка «откат/форк/btw» на живой строке обязана знать id записи.

**Решение.** Один режим id: `Session.Append*` возвращает id записи (или принимает заранее выданный id от вызывающего), и тот же id идёт в события `UserAppend`/`AssistantMessageUpdate`/`UserPromoted`; `session.Item.ID` живой строки равен `EntryID`. Замена `assistant-<nanos>` на id, полученный при старте хода из менеджера (или выданный менеджером заранее через `NextID`), — выбрать вариант с наименьшим числом мест изменения и без гонки между стримом и записью.

**Вне скоупа:** сами кнопки, откат, форк, aside.

**Сделано (2026-09-19).** Ветка `bug/live-transcript-entry-ids`, ворктри
`.worktrees/live-transcript-entry-ids`. Из двух вариантов взят тот, где id
минтит вызывающий. `llm.Message` получил host-поле `EntryID` рядом с
`DeliveryID`, и `Manager.appendMessage` пишет запись под ним. Сигнатуры
`Session.Append*` остались прежними. Id существует с первого токена стрима и
до записи в файл. Вариант с `NextID` отклонён: резервирование id без записи
открывает окно, в котором соседний `Append` возьмёт тот же id.

Поле `UserID` в `LoopOpts` раньше значило сразу две вещи, id строки и «строку
ещё надо нарисовать». Теперь id едет всегда, а рисование строки отделено
флагом `UserRowOwed`. `StartPrompt` возвращает id вызывающему, и сабмиттер
рисует строку под ним. Брифу субагента id выдаёт `assignmentPrompt`.

Регрессия закрыта `regcheck --base main`: тесты
`TestLiveRowIDsMatchReplayedEntries` и `TestAssignmentBriefRowNamesItsEntry`
краснеют на main и проходят на ветке.

**Засада.** Непустой id в контроллере значил «промпт ждёт доставки». По нему
отбирали, что вернуть в очередь при обрыве хода (`runLoop`, `requeueLocked`).
По нему же решали, что промотить из инжекта (`engine.go`). Id получили все
промпты, и бриф субагента пошёл доставляться по кругу. Поймал это
`TestAssignmentBriefOpensTheChildTranscript` десятиминутным таймаутом. Отбор
везде переведён на `rowOwed`. У `agent.InjectedPrompt` поле такое же. Тесты
`controller_prompt_test.go` и `children_followup_failure_test.go` уже
назывались словами «owed a row», им дописано поле.

**Ревью (review-medium).** Вернуло с двумя блокирующими. Первое совпало с
засадой выше. Второе про внешний контракт. `message_id` хука `post_turn` менял
формат с `assistant-<nanos>` на id записи. Правлен пример в `doc/hooks.md` и
строка CHANGELOG. Из неблокирующих взято всё. Занятый id теперь отказ вместо
тихой подмены (`manager.go`). Сабмиттер минтит id сам, когда контроллер
закрывается и отдаёт пустой. Дописаны тесты контракта `EntryID` в
`internal/session` и тест id брифа.

Второй круг ревью принял правку. Ревьювер дописал коммит `4f0c815`: в примере
`doc/hooks.md` стояло восемь знаков, а `NewEntryID` даёт шестнадцать, и автор
хука вывел бы из примера длину. Штатных повторов id, которые упёрлись бы в
новую ошибку, разбор путей не нашёл.

**На будущее.** Есть тонкое место, которое держится формой кода, а не
проверкой. Если `yield` с `UserPromoted` вернёт false, запись уже лежит в
логе, а промпт останется в `drained`, и повторная доставка упрётся в отказ по
занятому id. Сейчас недостижимо: для вступительного промпта это первое событие
хода, а для инжекта генератор до следующей укладки не доходит. Ломаться начнёт
при перекройке потребителя событий.

**Ручная проверка в TUI** (пункт 2 плана проверки) за пользователем.

## Acceptance Criteria

- После живого хода id строк транскрипта (user и assistant) равны id соответствующих записей в файле сессии
- Снимок транскрипта после живого хода и снимок после replay того же файла совпадают по id строк (тест)
- Tail-патч Mapper по-прежнему срабатывает на стриме (id assistant-строки стабилен от первого токена до конца хода)
- Строка в CHANGELOG под Unreleased

## Verification Plan

1. go build ./... и go test по изменённым пакетам: ./internal/agent/... ./internal/session/... ./internal/tui/submit/... ./internal/tui/controller/... ./internal/tui/transcript/...
2. Ручная проверка: ход в TUI, затем /context — id записей совпадают с id строк (временный лог или тест на Project)
3. Один scoped golangci-lint run по изменённым пакетам перед коммитом
