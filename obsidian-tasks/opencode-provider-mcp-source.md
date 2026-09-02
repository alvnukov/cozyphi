---
id: opencode-provider-mcp-source
title: Read providers and MCP servers from opencode
status: done
priority: high
model_level: high
task_type: feature
tags:
    - providers
    - mcp
    - opencode
    - config
acceptance_criteria:
    - Модели opencode отображаются как opencode/<provider>/<model> и используют API-ключ из auth.json без копирования
    - MCP-серверы opencode добавляются без префикса; при совпадении имени выигрывает cozyphi
    - Настройка opencode.enabled по умолчанию true отключает оба источника при false
    - make fmt-check lint test проходят
verification_plan:
    - Юнит-тесты адаптера на auth.json и opencode.json
    - Тесты приоритетов MCP и opencode.enabled
    - make fmt-check lint test
created_at: "2026-09-01T17:01:12.015213Z"
updated_at: "2026-09-01T21:06:29.870498Z"
---

## Body

**Контекст:** cozyphi должен читать настройки провайдеров и MCP-серверов opencode как второй read-only источник. OAuth и wellknown не импортируются. Файлы cozyphi не получают копий секретов opencode.

**Реализация:** добавить адаптер internal/opencode, встроить модели в контроллер и серверы в MCP loader, добавить включённую по умолчанию галочку в веб-настройках.

## Acceptance Criteria

- Модели opencode отображаются как opencode/<provider>/<model> и используют API-ключ из auth.json без копирования
- MCP-серверы opencode добавляются без префикса; при совпадении имени выигрывает cozyphi
- Настройка opencode.enabled по умолчанию true отключает оба источника при false
- make fmt-check lint test проходят

## Verification Plan

1. Юнит-тесты адаптера на auth.json и opencode.json
2. Тесты приоритетов MCP и opencode.enabled
3. make fmt-check lint test
