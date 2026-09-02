---
status: done
created_at: "2026-08-26T18:00:00.000000Z"
resolved_by: merge of fix/ctx-empty (61bb47e, 1ef6d24) → 9fea4e4
---

# ctx-empty-tool-rows

## Body

В /context сотни блоков показывались как «(empty)» — 228 из 591 в живой
сессии. Это assistant-сообщения с пустым content: они несут tool_calls
(tool_use-блоки) или reasoning. Пользователь видел их как «tool empty».

## Solution

`InspectContext` строит Body через `messageBody(msg)`: текст + по строке
`name {args}` на каждый tool-вызов (пустой текст — только вызовы), для
thinking-only поворотов — reasoning. Preview = previewLine(Body). Body —
только дисплейная проекция: модельный путь (EstimateMessageTokens, cut,
contextStats) не тронут.

## Acceptance Criteria

- [x] Assistant-поворот с tool_calls: Preview = `read {"path": …}`, Body = все вызовы
- [x] Текст + tool_calls: Body показывает и текст, и вызовы
- [x] Thinking-поворот: Body/Preview = reasoning
- [x] Scratch-прогон над живой сессией: 591 блок, 0 пустых (было 228)
- [x] Полный go test зелёный, lint 0, CHANGELOG

## Review findings (исправлены до мержа, 1ef6d24)

- Body-док переформулирован: дисплейная проекция, а не «ровно то, что видит
  модель» (reasoning не отправляется обратно в Anthropic)
- Поворот с текстом И tool_calls скрывал вызовы — теперь Body показывает оба
- Тест-претензия превью усилена до HasPrefix

## Residual

- OpenAI-совместимые провайдеры шлют reasoning_content обратно, Anthropic —
  нет; /context показывает reasoning у обоих (токены считают его у обоих).
