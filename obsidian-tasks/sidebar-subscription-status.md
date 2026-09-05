---
id: sidebar-subscription-status
title: 'Sidebar: состояние подписки провайдера в статусе'
status: done
priority: high
model_level: high
task_type: feature
tags:
    - sidebar
    - usage
    - provider
acceptance_criteria:
    - 'Во вкладке Status сайдбара после блока tokens и перед MCP есть секция subscription: план и по одной строке-бару на окно лимита с временем сброса; для провайдера без квоты секция отсутствует'
    - 'Данные приходят из существующего UsageQuotaMsg: сайдбар получает снимок в View наряду со statuspane/usagepane; загрузка стартует при старте сессии, после конца хода и после смены провайдера/учётки; устаревшие снимки другого провайдера не показываются'
    - Scoped-гейты по затронутым пакетам зелёные, CHANGELOG пополнен, задача закоммичена, смержена в main, worktree и ветка удалены
verification_plan:
    - go test -race ./internal/tui/sidebar/... ./internal/tui/sessions/... (и controller, если тронут)
    - gofmt -l по изменённым пакетам, golangci-lint fmt по изменённым пакетам
    - Один golangci-lint run по изменённым пакетам перед коммитом
    - Коммит в worktree, merge --no-ff в main, chore(tasks) коммит ledger, удаление worktree и ветки; без push
created_at: "2026-09-05T20:55:39.018348Z"
updated_at: "2026-09-05T21:10:19.516671Z"
---

## Body

**Что:** показать состояние текущей подписки провайдера (план, окна лимитов с процентом использования и временем сброса) во вкладке Status сайдбара. Место: после секции tokens, перед MCP — это данные об использовании, а не о рантайме.

**Источник данных:** provider.QuotaSnapshot приходит через controller.UsageQuotaMsg (FetchQuota/AcceptQuota с поколениями). View передаёт снимок в sidebar наряду со statuspane/usagepane. Сайдбар — dumb-виджет: SetQuota/ClearQuota/QuotaPolls, без I/O; ResetTarget в виджет не попадает.

**Триггеры загрузки:** старт View, RunEndedMsg, ProviderConnectResultMsg (после InvalidateQuota), смена провайдера (syncModelControls при SyncQuotaSelection=true — сброс снимка), плюс раз в минуту пока сайдбар виден — через wake отрисовки в View.Draw (quotaRefreshInterval), без отдельной горутины-тикера. Для Unsupported секция не рисуется и опрос не идёт.

**Готово (2026-09-06).** Реализация 299ec1f (Opus-агент, дифф проверен), merge в main fc8e9f8. FormatReset вынесен в internal/tui/tokens и используется usagepane и sidebar. Гейты по затронутым пакетам: gofmt, golangci-lint fmt, go build, go test -race (sidebar, sessions, usagepane, tokens), один golangci-lint run — 0 новых замечаний (4 старых в нетронутом коде оставлены). CHANGELOG и doc/tui.md обновлены. Push не делался.

## Acceptance Criteria

- Во вкладке Status сайдбара после блока tokens и перед MCP есть секция subscription: план и по одной строке-бару на окно лимита с временем сброса; для провайдера без квоты секция отсутствует
- Данные приходят из существующего UsageQuotaMsg: сайдбар получает снимок в View наряду со statuspane/usagepane; загрузка стартует при старте сессии, после конца хода и после смены провайдера/учётки; устаревшие снимки другого провайдера не показываются
- Scoped-гейты по затронутым пакетам зелёные, CHANGELOG пополнен, задача закоммичена, смержена в main, worktree и ветка удалены

## Verification Plan

1. go test -race ./internal/tui/sidebar/... ./internal/tui/sessions/... (и controller, если тронут)
2. gofmt -l по изменённым пакетам, golangci-lint fmt по изменённым пакетам
3. Один golangci-lint run по изменённым пакетам перед коммитом
4. Коммит в worktree, merge --no-ff в main, chore(tasks) коммит ledger, удаление worktree и ветки; без push
