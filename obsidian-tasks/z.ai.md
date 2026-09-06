---
id: z.ai
title: 'Z.AI: подписка — не отображаются значения (нули/устаревшие данные)'
status: done
priority: high
task_type: bug
branch: bug/z.ai
worktree_path: .worktrees/z.ai
verification_plan:
    - 'Диагноз: найти путь получения и отрисовки значений подписки z.ai, воспроизвести отказ с доказательствами'
    - Фикс в worktree на ветке задачи
    - Гейты только по изменённым пакетам
    - Мерж в main + ledger
created_at: "2026-09-06T11:15:08.473871Z"
updated_at: "2026-09-06T11:47:31.06292Z"
---

## Body

**Что:** пользователь видит нули/устаревшие данные подписки z.ai во всех местах, где она отображается.

**Где наблюдается:** везде (статус-бар, usage, настройки провайдера) — по ответам пользователя 2026-09-XX.

**Ожидание:** значения подписки (квоты/usage z.ai) актуальны и отображаются.

**Приёмка:** значения приходят и отображаются; причина найдена и задокументирована в ноте задачи.

**Done (2026-09-06).** Fixed and landed 2026-09-06. Root cause: z.ai API drift — limit windows (TOKENS_LIMIT/CREDIT_LIMIT) now carry only a used `percentage`, no usage/currentValue/remaining, which decoded as 0/0 budgets and rendered as zeros in every quota surface (status pane, /usage, sidebar). Fix in internal/provider/quota.go: zaiLimitAmounts falls back to zaiPercentAmounts when an absolute observation is absent; percent share rides UsedPercent (existing percent render path); missing/out-of-range share = no observation, not zero budget. TIME_LIMIT stays a sentinel. Regression test replays the captured live response (TestQuotaSnapshotZAIPercentageOnlyLimits). Landed: c8f0b06 on bug/z.ai, merged 76b6fe5 into main. Noted, not fixed: the fallback endpoint /api/monitor/usage now returns 404 (primary endpoint healthy, unreachable unless the primary starts rejecting keys).

## Verification Plan

1. Диагноз: найти путь получения и отрисовки значений подписки z.ai, воспроизвести отказ с доказательствами
2. Фикс в worktree на ветке задачи
3. Гейты только по изменённым пакетам
4. Мерж в main + ledger
