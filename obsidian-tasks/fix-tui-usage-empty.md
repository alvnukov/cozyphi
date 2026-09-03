---
id: fix-tui-usage-empty
title: /usage в TUI показывает пустые данные (квота/лимиты токенов без значений)
status: done
priority: high
task_type: bug
tags:
    - tui
    - bug
acceptance_criteria:
    - После хотя бы одного ответа модели /usage показывает ненулевые usage-числа (токены/квота), а не пустой контейнер
    - Есть unit-тест, покрывающий заполнение данных для /usage
    - make fmt-check lint test проходит
    - 'CHANGELOG.md: строка под [Unreleased]'
verification_plan:
    - Юнит-тест на заполнение данных /usage
    - make fmt-check lint test в worktree
    - Ручная проверка /usage пользователем в живой сессии
created_at: "2026-09-03T12:35:28.968097Z"
updated_at: "2026-09-03T14:22:16.265556Z"
---

## Body

**Symptom:** пользователь запускает /usage в TUI — приходит пустой контейнер; видны подписи про квоту/response/token limit, но без значений. Похоже, данные usage из ответов провайдера не долетают до рендера команды.

**Suspects:** парсинг usage из ответа API, агрегация session usage в engine/controller, маппинг в компонент /usage.

**Plan:** трассировка команды → фикс в минимальном шве → реализация в worktree через Opus-агента → gate → CHANGELOG.

**Done (2026-09-03).** Closed 2026-09-04: /usage failed with «quota response contains no token limits» because z.ai renamed TOKENS_LIMIT to CREDIT_LIMIT (usage=grant, currentValue=consumption, plan in data.level) and the legacy endpoint /api/monitor/usage/quota/limit rejects some valid keys — mirrored by openchamber#3012. Fix: internal/provider/quota.go decodeZAIQuota accepts both limit kinds via zaiLimitAmounts, plan chain gains data.level, fetchZAIQuota retries /api/monitor/usage on 401/API-rejection. 4 new tests. Gate green. Landed: 1a928fc merged as 6641abd on main.

## Acceptance Criteria

- После хотя бы одного ответа модели /usage показывает ненулевые usage-числа (токены/квота), а не пустой контейнер
- Есть unit-тест, покрывающий заполнение данных для /usage
- make fmt-check lint test проходит
- CHANGELOG.md: строка под [Unreleased]

## Verification Plan

1. Юнит-тест на заполнение данных /usage
2. make fmt-check lint test в worktree
3. Ручная проверка /usage пользователем в живой сессии
