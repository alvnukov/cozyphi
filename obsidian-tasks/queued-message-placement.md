---
id: queued-message-placement
title: Queued-сообщение рисуется в ленте и применяется не по порядку
status: done
priority: high
task_type: bug
tags:
    - tui
    - queue
    - transcript
branch: bug/queued-message-placement
worktree_path: .worktrees/queued-message-placement
acceptance_criteria:
    - Сообщение в очереди не появляется в транскрипте до доставки модели — висит виджетом над полем ввода
    - При доставке user-ряд добавляется в конец ленты (порядок совпадает с replay)
    - Drop при unapprove и Esc-recall не оставляют строк в ленте; recall возвращает text+media+skills в композер
    - Концепт Queued удалён из session/transcript (Message.Queued, Item.Queued, UserBlock.Queued, UserAppend.Queued, UserRecalled, спец-кейсы mapper.go и pane.go)
    - go build+test по изменённым пакетам зелёные
verification_plan:
    - go build ./internal/... && go test по пакетам session, tui/submit, tui/controller, tui/transcript, tui/composer, tui/sessions
    - 'Ручная проверка: сабмит во время стрима — сообщение над инпутом, не в ленте; конец хода — ряд в конце ленты; Esc — возврат в композер'
created_at: "2026-09-15T18:16:37.848486Z"
updated_at: "2026-09-15T21:05:00.000000Z"
---

## Body

**Симптом:** при сабмите во время работающего хода сообщение сразу рисуется в ленте с пометкой "(queued)" за стримящимся ходом, а при dequeue флаг снимается in-place — ряд остаётся выше событий, пришедших после (тулы, watch-события). Ожидается: сообщение висит над полем ввода, пока в очереди; в ленту попадает в конец при реальной отправке.

**Ревью (job_20260915T165808_84557c4bfdc9e3a2) нашло 5 дефектов одной хореографии:**
1. In-place promote (`internal/session/apply.go:71-77`) — главный баг порядка.
2. Drop при unapprove шлёт UserPromoted — ряд выглядит отправленным, при резюме исчезает (`internal/tui/controller/controller.go:2357,928`).
3. Гонка RunActive/StartPrompt → stuck "(queued)" (`internal/tui/submit/submitter.go:111-135`).
4. drainQueuedForRun обнуляет очередь до подтверждения доставки → strands rows при отмене.
5. Несогласованная семантика UserPromoted (mid-turn = «модель увидел», dequeue = «ран запустился», даже при отказе).

**Корень:** двойная запись о pending — promptQueue в контроллере и Queued-флаг в сессии, синхронизируемые вручную в 4 точках.

**Дизайн фикса (вариант B из ревью):**
- Удалить Queued-концепт: Message.Queued, Item.Queued, UserBlock.Queued, UserAppend.Queued, UserRecalled, спец-кейсы mapper.go:310/730/838, pane.go:578.
- Submitter не аппендит ряд при runActive; очередь рисуется виджетом над композером (зона pending-skills, composer/pane.go:510-517).
- UserPromoted несёт текст, становится append в конец ленты при доставке.
- Esc-recall = pop из очереди контроллера + перерисовка виджета; recall возвращает media/skills.
- Drain: requeue при недоставке или подтверждение движком.

**Started (2026-09-15).** Беру в работу: дизайн B по ревью — удаление Queued-концепта, виджет очереди над композером, promote = append в конец.

**Note (2026-09-15).** Design B implemented in worktree `.worktrees/queued-message-placement` (worker job + parent fixes): `Queued` concept removed from session/transcript (`Message.Queued`, `Item.Queued`, `UserBlock.Queued`, `UserAppend.Queued`, `UserRecalled`, mapper/pane special cases); queue lives only in controller `promptQueue` plus a new composer strip above the input (`PromptQueueMsg`); `UserPromoted{ID, Text}` is now emitted by the engine at real delivery (turn start and inject boundary) and appends the user row at the END of the feed, matching replay. Also fixed by parent: `StartPrompt` returns `(queued bool)` under `streamMu`; submitter skips the transcript row for queued submits; `dropQueuedPromptsLocked` no longer fakes `UserPromoted` on unapprove; drained-but-undelivered prompts are requeued on cancel; engine no longer double-publishes `UserPromoted` in the run loop. Recall restores text+media+pendingSkills. Tests rewritten; CHANGELOG entry added. Pending: full scoped test run, task note commit, one scoped golangci-lint run, branch commit.

**Done (2026-09-15).** Landed on branch `bug/queued-message-placement` in worktree `.worktrees/queued-message-placement` (commits b5e76b98 + 137da947, not pushed). Design B: queued prompts no longer get a transcript row at submit — they wait in the controller queue, mirrored as a `queued: … (Esc to recall)` strip above the input; the engine emits `UserPromoted{ID, Text}` at real delivery (turn start / inject boundary) and the row appends at the feed end, matching replay. Removed `Queued` everywhere (session/transcript), `UserRecalled`, fake promotes on unapprove; recall restores text+media+skills; undelivered drains are requeued on cancel. Gates: scoped build+tests green (session, agent, controller, submit, composer, transcript, sessions), gofmt clean, one scoped golangci-lint run — 0 issues.

**Review fixes (2026-09-15).** Двухосевое ревью PR #32 (Standards + Spec) нашло, что запись «Done» выше была преждевременной. Исправлено на той же ветке:
- CI-блокер: `cmd/session_child_close_test.go:152,158` не компилировались после того, как `StartPrompt` потерял параметр `userID`. Прошлый прогон гейтов был `go build ./...` (не компилирует тестовые файлы) плюс тесты только по `internal/*` — поэтому дыра.
- Первый (снятый с очереди) промпт терялся без следа: `drained`/`requeueUndelivered` покрывали только mid-turn инъекции, а каждый ранний выход из `runLoop`/`startPromptLocked` выбрасывал головной промпт молча. Теперь он засеивается в `drained`, а `requeueLocked` возвращает его в начало очереди на всех путях отказа.
- Ход, закончившийся ошибкой, теперь ничего не возвращает в очередь (`fail()`): иначе `finishRun` запускал бы тот же падающий ход бесконечно. Возврат остаётся только для оборванного хода (Esc, вытеснение).
- Сабмит в удержанного (terminal) ребёнка не попадал ни в ленту (submitter пропускал ряд), ни в `promptQueue` (полоса пустая), а при неудачном спавне терялся совсем. Теперь на всех путях отказа follow-up (`followup.go`, `assignment.go`) промпт с id возвращается в очередь — оттуда его видно и можно забрать по Esc; ряд по-прежнему рисует движок в момент доставки.
- Ветка перебазирована на main (в неё успели влиться #31, #33, #34, #27). Конфликты: CHANGELOG и `assignment.go` — из #33, который открывает экран сабагента брифом. Совместили: `publishAssignmentBriefLocked` печатает бриф только для собранного первого сообщения (`id == ""`), а у набранного человеком follow-up id есть, и ряд ставит движок при доставке — дубля нет.
- Полоса очереди над инпутом была неограниченной и через `MinHeight()` выталкивала инпут за экран (проверено прогоном: 14 промптов на 24 строках). Введён `maxQueuedRows = 4` с добивкой `queued: N more`; магическая `width-28` заменена на `queuedChromeWidth`.
- Мелочи: `UserPromptDisplayText` переехал из `agent` в `session.UserPromptText` (submitter больше не импортирует UI→agent), исправлен ложный комментарий в `startNextLocked`, вычищены устаревшие упоминания queued-ряда в `mapper.go` и `internal/tui/DESIGN.md`, `newUserMessage(n)` → `count`.
Гейты: `go build ./...`, тесты по session/agent/tui/*/components/chat/components/block/cmd — зелёные; один scoped `golangci-lint run` — 0 issues. Добавлены регрессии: возврат промпта на оборванном ходе, отсутствие возврата на упавшем, фильтр `requeueLocked` по id, ряд follow-up у удержанного ребёнка, потолок полосы очереди.

## Acceptance Criteria

- Сообщение в очереди не появляется в транскрипте до доставки модели — висит виджетом над полем ввода
- При доставке user-ряд добавляется в конец ленты (порядок совпадает с replay)
- Drop при unapprove и Esc-recall не оставляют строк в ленте; recall возвращает text+media+skills в композер
- Концепт Queued удалён из session/transcript (Message.Queued, Item.Queued, UserBlock.Queued, UserAppend.Queued, UserRecalled, спец-кейсы mapper.go и pane.go)
- go build+test по изменённым пакетам зелёные

## Verification Plan

1. go build ./internal/... && go test по пакетам session, tui/submit, tui/controller, tui/transcript, tui/composer, tui/sessions
2. Ручная проверка: сабмит во время стрима — сообщение над инпутом, не в ленте; конец хода — ряд в конце ленты; Esc — возврат в композер
