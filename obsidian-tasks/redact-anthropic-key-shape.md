---
id: redact-anthropic-key-shape
title: redact не маскирует ключи вида sk-ant-api03-…
status: todo
priority: high
model_level: medium
task_type: bug
tags:
    - security
acceptance_criteria:
    - Ключи форм sk-ant-api03-… и sk-ant-… маскируются целиком, без остатка тела в выводе.
    - Kebab-case проза и slug вида task-sk-v2-… не маскируются; Redact остаётся идемпотентной.
    - Тесты в internal/redact покрывают реальные префиксы провайдеров, которые встречаются в конфиге продукта.
verification_plan:
    - Табличный тест internal/redact на реальные и ложные формы.
    - Scoped go test ./internal/redact/ и пакетов, которые наследуют маску.
created_at: "2026-09-07T01:18:31.937291Z"
updated_at: "2026-09-07T01:18:31.937291Z"
---

## Body

**Что не так.** `internal/redact` маскирует OpenAI-подобные ключи правилом `sk-(proj-)?[A-Za-z0-9]{20,}`, но тело ключа Anthropic содержит дефисы: в `sk-ant-api03-XXXX…` после `sk-` идёт `ant` (три символа) и правило не срабатывает. То есть основной credential этого продукта проходит через маску нетронутым. Обнаружено при написании тестов аудита в задаче developer-mode-diagnostics: строка `sk-ant-api03-AAAA…` вернулась из `Redact` как есть.

**Почему это важно.** Маску наследуют все downstream-поверхности: durable plan, projection, full view, sidebar, receipt, audit и boundary `internal/diag`. Ключ, приехавший из tool output или прозы, окажется в persisted plan и в harness-ответах.

**Что учесть.** Узость правил намеренна — комментарий в `redact.go` объясняет, что дефисы внутри тела дали бы false positive на kebab-slug вроде `task-sk-v2-…`. Нужна форма, привязанная к известному префиксу (`sk-ant-`), а не общее разрешение дефисов. Проверить заодно остальные реальные префиксы, которые продукт видит: `sk-ant-api03-`, `sk-ant-`, ключи провайдеров из config.

**Проверка.** Таблица в `internal/redact/redact_test.go`: реальные формы маскируются, kebab-slug и обычная проза не трогаются, `Redact` остаётся идемпотентной.

## Acceptance Criteria

- Ключи форм sk-ant-api03-… и sk-ant-… маскируются целиком, без остатка тела в выводе.
- Kebab-case проза и slug вида task-sk-v2-… не маскируются; Redact остаётся идемпотентной.
- Тесты в internal/redact покрывают реальные префиксы провайдеров, которые встречаются в конфиге продукта.

## Verification Plan

1. Табличный тест internal/redact на реальные и ложные формы.
2. Scoped go test ./internal/redact/ и пакетов, которые наследуют маску.
