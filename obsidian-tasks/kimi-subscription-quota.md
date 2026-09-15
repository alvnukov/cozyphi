---
id: kimi-subscription-quota
title: 'Kimi: состояние подписки (quota-адаптер /usages)'
status: in_progress
priority: high
task_type: feature
parent_id: kimi-subscription-provider
tags:
    - kimi
    - provider
    - quota
branch: feature/kimi-subscription-quota
worktree_path: .worktrees/kimi-subscription-quota
acceptance_criteria:
    - quotaAdapters содержит kimi-code → fetchKimiQuota
    - Адаптер ходит в {cred.BaseURL}/usages с Bearer access, декодит 4 usage-окна (used_ratio→percent, reset_time ISO) и booster wallet
    - 'Тесты на httptest: happy path, 401/404, невалидный JSON, не-подключённый провайдер'
    - Scoped гейты зелёные, CHANGELOG, подписанный коммит на ветке feature/kimi-subscription-provider
verification_plan:
    - go build/vet/test ./internal/provider/...
    - один scoped golangci-lint run ./internal/provider/...
    - CHANGELOG [Unreleased]
created_at: "2026-09-15T12:14:44.219931Z"
updated_at: "2026-09-15T12:14:51.063776Z"
---

## Body

**What:** добавить kimi-code в квота-слой подписки (Manager.QuotaSnapshot / quotaAdapters в internal/provider/quota.go) по аналогии с openai (codex) и zai-coding-plan.

**Facts confirmed by first-hand source** (MoonshotAI/kimi-code, packages/oauth/src/managed-usage.ts, клон /tmp/kimi-code-src, shallow 2026-09-15): endpoint GET {base}/usages на api.kimi.com/coding/v1, заголовки Authorization: Bearer <access token>, Accept: application/json. Пейлоад snake_case: usages.{limit_5h,limit_7d,limit_month_total,limit_month_code} = {used_ratio: number (0..1), reset_time: ISO8601 string optional}; booster_wallet nullable: {balance:{type:"BOOSTER",amount,amountLeft} — amount фикс-пойнт, центы×1_000_000; monthly_charge_limit:{priceInCents,currency}; monthly_used:{priceInCents,currency}; monthly_charge_limit_enabled: bool}. Ошибки в клиенте: 401 → "Authorization failed", 404 → "Usage endpoint not available. Try Kimi For Coding."

**Design:** новый файл internal/provider/kimi_quota.go (по образцу kimi_oauth.go/kimi_models.go): fetchKimiQuota — Bearer-помощник из kimi_oauth.go (requestWithinBaseURL, если подходит), окна → QuotaLimit{Unit:"percent", UsedPercent=ratio*100} с Window-лейблами "5 hours"/"7 days"/"month"/"month · code" (по образцу codex scope·label), reset_time → ResetsAt. Booster wallet → один QuotaLimit с Unit=валюта (по аналогии с z.ai credits), только когда amount > 0. Регистрация в quotaAdapters: kimi-code → fetchKimiQuota. Менеджер уже рефрешит oauth-креденшел до адаптера и проверяет connected — повторно не дублировать. monthly charge limit в UI не выносить (минимализм, в комментарии).

**Done:** /usage (или где рендерится QuotaSnapshot) показывает состояние kimi-подписки; гейты scoped; CHANGELOG; коммит на feature/kimi-subscription-provider.

**Started (2026-09-15).** Взял задачу; работаю в воркгруте .worktrees/kimi-subscription-provider на ветке feature/kimi-subscription-provider (та же ветка, что и parent-задача).

## Acceptance Criteria

- quotaAdapters содержит kimi-code → fetchKimiQuota
- Адаптер ходит в {cred.BaseURL}/usages с Bearer access, декодит 4 usage-окна (used_ratio→percent, reset_time ISO) и booster wallet
- Тесты на httptest: happy path, 401/404, невалидный JSON, не-подключённый провайдер
- Scoped гейты зелёные, CHANGELOG, подписанный коммит на ветке feature/kimi-subscription-provider

## Verification Plan

1. go build/vet/test ./internal/provider/...
2. один scoped golangci-lint run ./internal/provider/...
3. CHANGELOG [Unreleased]
