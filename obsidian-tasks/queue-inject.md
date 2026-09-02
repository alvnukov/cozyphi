---
status: done
created_at: "2026-08-26T19:00:00.000000Z"
resolved_by: merge of fix/queue-inject (f4a0d1e, c6ae5ce, 9928a27) → 270758c
---

# queue-inject

## Body

Сообщение, отправленное во время работающей модели, показывалось в
транскрипте и вставало в очередь (как в Claude Code/opencode) — но модель
на него не отвечала: редьюсер ломал стриминговый поворот.
`assistantReplaceIndex` требовал, чтобы последней строкой был ассистент;
`UserAppend` во время стриминга вставлял user-строку в хвост, завершающий
апдейт не находил свой поворот → дубль ассистента + исходная строка навсегда
в `StateStreaming`.

## Solution

`assistantReplaceIndex`:
1. сначала ищет активный (streaming) ассистентский поворот хвостом назад —
   впитывает апдейты, даже если ниже вставлен queued user-мессаж;
2. иначе матчит последнего ассистента по ID (не последнюю строку);
3. иначе — новый поворот.

Очередь в Controller уже работала (StartPrompt → promptQueue → finishRun);
интеграционный прогон подтвердил 2 промпта → 2 запроса к модели.

## Acceptance Criteria

- [x] UserAppend во время стриминга: нет дубля и ghost; `[u1, a1(complete), u2]`, IsStreaming=false
- [x] Очередной промпт отвечает после stop (интеграционно)
- [x] Late same-ID update заменяет поворот даже при user-строке ниже
- [x] Существующие тесты Apply/Streaming зелёные; полный go test exit=0; lint 0

## Review findings (исправлены до мержа, 9928a27)

- Док `AssistantMessageUpdate` переписан под новое правило (streaming turn + queued user)
- Idle-fallback теперь матчит последнего **ассистента** по ID, а не последнюю строку
- Регресс-тест `TestApplySameIDUpdateAfterQueuedUserAppend` добавлен

## Residual

- Nit из ревью (pre-existing, не из этого diff): `markProjectionChange` теряет
  хвостовой fast-path, пока queued user-строка лежит под стриминговым
  ассистентом (полный re-sync за окно) — корректность не страдает.
- Nit (pre-existing): `findCoalesceSession` может схлопнуть два
  `AssistantMessageUpdate` с разными ID при >120ms задержке UI-дрейна.
