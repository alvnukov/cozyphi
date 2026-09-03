---
id: fix-transcript-scroll-ux
title: 'Транскрипт: якорь прокрутки при стриме + автоскролл выделения у кромки'
status: done
priority: high
task_type: bug
tags:
    - phase1
    - ux
    - transcript
verification_plan:
    - go test ./internal/components/transcript/ ./internal/tui/transcript/ ./internal/tui/editor/
    - make fmt-check lint test
    - 'ручная проверка: стрим при отмотке — вид стоит; выделение за кромку — ускоренная прокрутка'
created_at: "2026-08-24T16:57:42.390457Z"
updated_at: "2026-08-24T17:08:45.808347Z"
---

## Body

Два бага прокрутки транскрипта (репорт пользователя):

1. Выделение мышью останавливается у нижней кромки: при достижении низа
   список не прокручивается — нельзя выделить несколько экранов. Нужна
   автопрокрутка с градиентом скорости: немного выше кромки — медленно,
   на кромке — быстрее, за кромкой (зона композера/футера, Y >= listH) —
   ещё быстрее с набором скорости по глубине. Прокрутка продолжается,
   пока кнопка удерживается (тики отрисовки ~50мс через ctx.WakeIn,
   паттерн спиннера в Editor.Draw), выделение тянется за прокруткой,
   у краёв контента останавливается. Верхняя кромка — зеркально.

2. При отмотке вверх (ScrollFromBottom > 0) стрим модели дёргает экран:
   MessageList заякорен к низу, рост контента на Δ сдвигает вид на Δ.
   Фикс: при ScrollFromBottom > 0 рост totalH увеличивает отступ на Δ
   (якорь по верхней видимой строке); follow mode (0) без изменений.

Дизайн: MessageList.ScrollBy(rows) int (клампит, возвращает факт);
wheel/PageUp/PageDown переводятся на него. TranscriptPane:
edgeScrollZone(y) → (dir, step); AdvanceEdgeScroll() bool — шаг тика;
Editor.Draw вызывает и делает WakeIn(edgeScrollInterval).

Тесты: message_list growth keeps top anchor (буквенные высоты);
pane: зоны у кромки/за кромкой/наверху, истощение, extеnsion ey.

## Verification Plan

1. go test ./internal/components/transcript/ ./internal/tui/transcript/ ./internal/tui/editor/
2. make fmt-check lint test
3. ручная проверка: стрим при отмотке — вид стоит; выделение за кромку — ускоренная прокрутка
