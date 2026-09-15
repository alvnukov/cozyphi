---
id: kimi-subscription-quota
title: 'Kimi: состояние подписки (quota-адаптер /usages)'
status: done
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
updated_at: "2026-09-15T12:43:41.998199Z"
---

## Body

**What:** добавить kimi-code в квота-слой подписки (Manager.QuotaSnapshot / quotaAdapters в internal/provider/quota.go) по аналогии с openai (codex) и zai-coding-plan.

**Facts confirmed by first-hand source** (MoonshotAI/kimi-code, packages/oauth/src/managed-usage.ts, клон /tmp/kimi-code-src, shallow 2026-09-15): endpoint GET {base}/usages на api.kimi.com/coding/v1, заголовки Authorization: Bearer <access token>, Accept: application/json. Пейлоад snake_case: usages.{limit_5h,limit_7d,limit_month_total,limit_month_code} = {used_ratio: number (0..1), reset_time: ISO8601 string optional}; booster_wallet nullable: {balance:{type:"BOOSTER",amount,amountLeft} — amount фикс-пойнт, центы×1_000_000; monthly_charge_limit:{priceInCents,currency}; monthly_used:{priceInCents,currency}; monthly_charge_limit_enabled: bool}. Ошибки в клиенте: 401 → "Authorization failed", 404 → "Usage endpoint not available. Try Kimi For Coding."

**Design:** новый файл internal/provider/kimi_quota.go (по образцу kimi_oauth.go/kimi_models.go): fetchKimiQuota — Bearer-помощник из kimi_oauth.go (requestWithinBaseURL, если подходит), окна → QuotaLimit{Unit:"percent", UsedPercent=ratio*100} с Window-лейблами "5 hours"/"7 days"/"month"/"month · code" (по образцу codex scope·label), reset_time → ResetsAt. Booster wallet → один QuotaLimit с Unit=валюта (по аналогии с z.ai credits), только когда amount > 0. Регистрация в quotaAdapters: kimi-code → fetchKimiQuota. Менеджер уже рефрешит oauth-креденшел до адаптера и проверяет connected — повторно не дублировать. monthly charge limit в UI не выносить (минимализм, в комментарии).

**Done:** /usage (или где рендерится QuotaSnapshot) показывает состояние kimi-подписки; гейты scoped; CHANGELOG; коммит на feature/kimi-subscription-provider.

**Started (2026-09-15).** Взял задачу; работаю в воркгруте .worktrees/kimi-subscription-provider на ветке feature/kimi-subscription-provider (та же ветка, что и parent-задача).

**Done (2026-09-15).** Сделано (2026-09-15) на ветке feature/kimi-subscription-provider, коммит c6ac6cac (подписан). internal/provider/kimi_quota.go: fetchKimiQuota — GET {cred.BaseURL}/usages (контракт подтверждён первоисточником MoonshotAI/kimi-code packages/oauth/src/managed-usage.ts), авторизация переиспользует kimiGrant.authorize (Bearer + confinement). Декод: 4 окна (limit_5h/7d/month_total/month_code, used_ratio number|string → UsedPercent, reset_time RFC3339 → ResetsAt), booster wallet → валютный лимит «top-up wallet» (фикс-пойнт центы ×1e6, только type=BOOSTER и amount>0; monthly charge limit knobs в UI не выносились — минимализм). Регистрация: quotaAdapters["kimi-code"]. Тесты kimi_quota_test.go на httptest: happy path, string-ratio, пустой ответ, не-booster wallet, 401/404/invalid JSON/disconnected, api-key refusal (7 сценариев). Гейты scoped: build/vet/test internal/provider зелёные, golangci-lint run 0 issues. CHANGELOG [Unreleased]. Живой прогон против api.kimi.com не делался (нужен реальный аккаунт) — окна появятся в usage-view автоматически через существующий QuotaSnapshot-контракт.

**Note (2026-09-15).** **Bugfix (2026-09-15).** Usage view показывал «subscription unavailable»: живой пейлоад api.kimi.com/coding/v1/usages (снят curl'ом с oauth-токеном) не совпал с декодом. Два расхождения: (1) booster-кошелёк приезжает camelCase (`boosterWallet`, `balance.amountLeft`, `monthlyChargeLimit`, `monthlyUsed`, `monthlyChargeLimitEnabled`) — как в parseBoosterWallet первоисточника, а теги были snake_case, из-за чего wallet молча пропадал; (2) для аккаунта без managed-видa endpoint возвращает generic-форму `limits:[{window:{duration,timeUnit},detail:{limit,used,resetTime}}]` (строковые числа), которой адаптер не знал вовсе → «no usage windows». Фикс: теги кошелька camelCase; форма limits принята как fallback (managed-окна выигрывают при наличии обеих); лейблы окон из duration+TIME_UNIT_* («300 TIME_UNIT_MINUTE» → «5 hours», неизвестные юниты — wire-токен). Тесты: фикстуры приведены к форме первоисточника/живого ответа (урок: фикстуры, скопированные с собственного кода, wire не валидируют) + новый TestQuotaSnapshotKimiGenericRateLimitShape.

## Acceptance Criteria

- quotaAdapters содержит kimi-code → fetchKimiQuota
- Адаптер ходит в {cred.BaseURL}/usages с Bearer access, декодит 4 usage-окна (used_ratio→percent, reset_time ISO) и booster wallet
- Тесты на httptest: happy path, 401/404, невалидный JSON, не-подключённый провайдер
- Scoped гейты зелёные, CHANGELOG, подписанный коммит на ветке feature/kimi-subscription-provider

## Verification Plan

1. go build/vet/test ./internal/provider/...
2. один scoped golangci-lint run ./internal/provider/...
3. CHANGELOG [Unreleased]
