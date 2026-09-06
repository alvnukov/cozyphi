---
id: developer-mode-readonly
title: 'Developer mode: полное read-only наблюдение за cozyphi'
status: todo
priority: medium
model_level: medium
task_type: epic
tags:
    - developer-mode
    - goal
acceptance_criteria:
    - Завершены все 17 дочерних задач; каждая категория доступна через harness, без незаявленных пробелов.
    - Developer mode включается только явным --developer-mode в TUI/headless; текущий процесс не эскалирует полномочия, дочерние агенты их не наследуют.
    - Инструмент полностью read-only и проходит обычные tool loop, permission gate и ограничения плана; специального developer-launcher или дополнительного согласия на запуск нет.
    - Все категории имеют безопасное представление configured/loaded/effective, источников, доступности и условий применения; секреты не попадают в ответы, ошибки и audit.
    - Проверки ограничены затронутыми пакетами/сценариями; опубликована документация реализованного контракта.
verification_plan:
    - Проверить граф 17 задач и закрытие критериев каждой категории.
    - Выполнить профильную матрицу TUI/headless, redaction, lifecycle и side-effect-free snapshot.
    - Сопоставить документацию с проверенным поведением; зафиксировать обоснованные исключения без обещаний неподдержанной диагностики.
created_at: "2026-09-06T09:05:14.345036Z"
updated_at: "2026-09-06T09:05:14.345036Z"
---

## Body

**Согласованный результат.** Полное read-only наблюдение агентом за cozyphi. Активация только явным --developer-mode; config, ENV, UI и resume не включают режим. Запуск cozyphi агентом не требует специального разрешения сверх обычного permission gate. Нет write-capabilities, set, reload, reset, compaction, launcher или защиты от same-UID shell.

**Interface.** Нативный harness с action catalog/snapshot/explain. Catalog показывает все категории и доступность; snapshot без фильтра выдаёт краткий обзор, с фильтром — ограниченные детали; explain объясняет одно поддержанное поле. Категории: runtime, model, context, permissions, plan, tools, integrations, agents, storage, ui, diagnostics. Watch-метаданные относятся к diagnostics, роли/задания к agents; credentials/provider metadata к model.

**Модель данных.** Для параметра различать configured (задано в источнике), loaded (загружено владельцем), effective (действует сейчас), provenance каждого слоя, scope, observed_at, revision владельца и apply semantics: immediate/next_turn/reload/new_session/restart. Различать unset/redacted/unavailable/not_applicable и несвежий снимок. Не угадывать provenance; сначала допустим явный unavailable, но финальный аудит обязан закрыть пробелы или перечислить обоснованные исключения. Результат detached; снимки разных владельцев не глобально атомарны.

**Архитектурное решение.** Один UI-независимый модуль диагностики с маленьким interface, каталогом, DTO, ограничениями и redaction. Владельцы состояния остаются прежними; TUI/headless подключают одинаковый контракт. Не делать массовый рефакторинг /status обязательным условием. Инструмент регистрируется до plan-step injection; runtime проверяет отсутствие developer-полномочий даже при прямом вызове. Общие process/workspace ресурсы не дают дочернему агенту доступ.

**Безопасность.** Экспорт только allowlisted полей, не raw config/model structs, ENV, transcript, prompts, memory contents, логи, профили, команды hooks/watches, MCP args/env или схемы серверных инструментов. Секреты — presence/source без значений, хешей и суффиксов. Пути, URL, произвольные строки и ошибки тоже очищаются до любого ответа/audit. Чтение не выполняет hooks, сетевые пробы, запуск процессов, reload или сканирование истории. Audit — безопасные метаданные действия, scope, результата и частичности, не payload. Новые UI-элементы для аудита не нужны.

**Порядок.** 01 headless; 02 TUI; 03 model; 04 providers после 03; 05 permissions; 06 tools после 05; 07 context; 08 plan; 09 MCP; 10 LSP; 11 hooks; 12 agents/watches; 13 storage; 14 UI после 02; 15 diagnostics; 16 coverage после 02–15; 17 docs после 16. Остальные категории зависят только от 01. Blocked by: None — контейнер, не исполнительская задача. Исполнители читают этот контракт вместе со своей задачей и работают по frontier; blocked-состояние снимается после завершения всех названных зависимостей.

**Работа.** Все дочерние задачи medium/low. Код только в task worktree, не main. Каждая задача демонстрирует полный путь от реального владельца до harness и включает свои тесты. Узкие проверки, без общерепозиторных Go gates. Закрытие реализации: code commit → merge --no-ff → отдельный ledger commit → cleanup; push только по просьбе. Epic не закрывать до приёмки всех детей.

## Acceptance Criteria

- Завершены все 17 дочерних задач; каждая категория доступна через harness, без незаявленных пробелов.
- Developer mode включается только явным --developer-mode в TUI/headless; текущий процесс не эскалирует полномочия, дочерние агенты их не наследуют.
- Инструмент полностью read-only и проходит обычные tool loop, permission gate и ограничения плана; специального developer-launcher или дополнительного согласия на запуск нет.
- Все категории имеют безопасное представление configured/loaded/effective, источников, доступности и условий применения; секреты не попадают в ответы, ошибки и audit.
- Проверки ограничены затронутыми пакетами/сценариями; опубликована документация реализованного контракта.

## Verification Plan

1. Проверить граф 17 задач и закрытие критериев каждой категории.
2. Выполнить профильную матрицу TUI/headless, redaction, lifecycle и side-effect-free snapshot.
3. Сопоставить документацию с проверенным поведением; зафиксировать обоснованные исключения без обещаний неподдержанной диагностики.
