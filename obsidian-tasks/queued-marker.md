---
status: done
created_at: "2026-08-26T19:30:00.000000Z"
resolved_by: merge of fix/queued-marker (b65f2e7, dbf6444, c01fac6, 2ebb0c8)
---

# queued-marker

## Body

Сообщение, отправленное во время работающей модели, ставилось в очередь и
видно в транскрипте — но ничем не помечалось, что оно ждёт остановки модели.
Нужен маркер «(queued)», как в Claude Code/opencode.

## Solution

Флаг `Queued` прокинут по цепочке отображения:
`session.UserAppend` → `Message` → `Project.Item` → `Mapper.UserBlock` →
`block.UserBlock.Draw` (маркер «(queued)» под текстом, CopyText чистый).
Submitter выставляет флаг при `Controller.RunActive()`.

Очистка маркера (review-fix): `queuedPrompt` несёт id строки транскрипта
(`session.NewUserMessageID()`), при dequeue в `finishRun` публикуется
`session.UserPromoted{ID}`, который снимает `Queued` с нужной user-строки.
Тот же путь используется при `SetPlanApproved(false)` (drop очереди).

## Acceptance Criteria

- [x] Submit во время активного run помечает user-строку как queued
- [x] Проекция/блок рисуют маркер «(queued)», копирование текста чистое
- [x] Idle-субмит остаётся обычной user-строкой без маркера
- [x] Маркер исчезает в момент dequeue (UserPromoted) и при drop очереди (unapprove)
- [x] Полный go test exit=0; lint 0

## Review findings

- (major) маркер не снимался навсегда → `UserPromoted` + id через очередь (c01fac6)
- (must-fix) drop очереди при unapprove оставлял stale-подсказки → `dropQueuedPromptsLocked` (2ebb0c8)
- (minor, residual) маркер попадает в drag-selection копирование — строку «(queued)»
  можно скопировать выделением; whole-block copy чистый (`CopyText`). Оставлено как noise.

## Residual

- Нет счётчика «N queued» и меню управления очередью (edit/remove/send now) —
  как у opencode. Отдельная задача, если потребуется.
- `Cancel()` (Esc) сохраняет принятые промпты и auto-drain продолжает очередь;
  opencode в этом случае паузит очередь (`followup.paused`). Кандидат на отдельную задачу.
