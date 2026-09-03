---
id: agents-md-cozyphi-principles
title: 'AGENTS.md: выкинуть принципы phi, записать принципы CozyPhi'
status: done
priority: medium
task_type: docs
parent_id: cozyphi-enterprise-code-review
tags:
    - docs
    - agents-md
acceptance_criteria:
    - AGENTS.md говорит от лица CozyPhi, не phi
    - Каждая из шести осей качества зафиксирована
    - Инварианты кода сохранены
    - Нет дублей того, что несет среда (Makefile, CI)
verification_plan:
    - прочитать итоговый файл
    - git diff показывает только AGENTS.md
created_at: "2026-08-23T14:58:37.988669Z"
updated_at: "2026-08-23T14:59:31.673958Z"
---

## Body

Переработка AGENTS.md под форк: миссия CozyPhi (удобный не-минималистичный агент, фаза 1 UI, фаза 2 cozy-tools), энтерпрайз-стандарты качества (архитектура/расширяемость/тестируемость/безопасность/надежность/читаемость), догфудинг mcp-ai-helper (реестр задач, заметки). Убрать phi-специфику: позиционирование minimal, история миграции с китайского, ссылки на процесс pulseaiclub. Сохранить нагрузочные инварианты кода (порядок tool loop, hashline edit, изоляция суб-агентов, UI split, lean deps).

## Acceptance Criteria

- AGENTS.md говорит от лица CozyPhi, не phi
- Каждая из шести осей качества зафиксирована
- Инварианты кода сохранены
- Нет дублей того, что несет среда (Makefile, CI)

## Verification Plan

1. прочитать итоговый файл
2. git diff показывает только AGENTS.md
