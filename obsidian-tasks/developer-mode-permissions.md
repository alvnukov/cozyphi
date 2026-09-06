---
id: developer-mode-permissions
title: 05 — Объяснить действующую permission policy без её изменения
status: done
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - harness permissions показывает действующую gate policy, bypass, ограничения и источники отличий от loaded/configured.
    - Чувствительные literals правил не экспортируются; безопасные rule IDs, типы, counts и redacted metadata позволяют объяснить ограничения.
    - Developer mode не меняет результат обычной permission policy и не обходит hooks/plan; диагностический запрос сам проходит gate.
    - Не выполняются команды, path mutations или разрешительные пробы ради проверки policy; read не выдаёт новые grants.
    - Неизвестный/decorated gate возвращает обоснованный unavailable, а не AllowAll/default.
verification_plan:
    - 'Table tests обычного/headless/bypass/decorated gate: configured и effective различаются правильно.'
    - Secret literals в правилах/путях не попадают в result/errors/audit.
    - Spies Ask/Run/hooks подтверждают отсутствие probe execution; обычный tool-loop hook самого запроса остаётся как раньше.
created_at: "2026-09-06T09:07:38.76507Z"
updated_at: "2026-09-06T12:10:31.787128Z"
---

## Body

**Что построить.** Безопасное объяснение фактически действующей политики доступа через harness. Читать epic developer-mode-readonly.

**Blocked by:** developer-mode-headless.

**Фиксированные решения.** Наблюдать уже собранный gate и его wrappers; нельзя считать project permissions фактической политикой после bypass/headless преобразования. Не вызывать интерактивный Ask или Run для диагностики и не симулировать разрешение произвольного shell. Представление rules allowlisted; paths/patterns могут содержать секреты. Коллизии с обнаруженными существующими security bugs не обходить: отдельная bug-задача и остановка только реально зависимой части.

**Scope.** Policy metadata, происхождение и safe explanation. Возможность конкретного инструмента в текущем плане делает developer-mode-tools.

**Работа.** Task worktree, узкие тесты, closeout по epic.

## Acceptance Criteria

- harness permissions показывает действующую gate policy, bypass, ограничения и источники отличий от loaded/configured.
- Чувствительные literals правил не экспортируются; безопасные rule IDs, типы, counts и redacted metadata позволяют объяснить ограничения.
- Developer mode не меняет результат обычной permission policy и не обходит hooks/plan; диагностический запрос сам проходит gate.
- Не выполняются команды, path mutations или разрешительные пробы ради проверки policy; read не выдаёт новые grants.
- Неизвестный/decorated gate возвращает обоснованный unavailable, а не AllowAll/default.

## Verification Plan

1. Table tests обычного/headless/bypass/decorated gate: configured и effective различаются правильно.
2. Secret literals в правилах/путях не попадают в result/errors/audit.
3. Spies Ask/Run/hooks подтверждают отсутствие probe execution; обычный tool-loop hook самого запроса остаётся как раньше.
