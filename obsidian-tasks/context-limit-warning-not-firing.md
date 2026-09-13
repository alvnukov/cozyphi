---
id: context-limit-warning-not-firing
title: Не срабатывают предупреждение и отключение инструментов при превышении контекста
status: done
priority: high
task_type: bug
branch: bug/context-limit-warning-not-firing
worktree_path: .worktrees/context-limit-warning-not-firing
verification_plan:
    - Тест, воспроизводящий несрабатывание, красный до фикса и зелёный после
    - go build + go test по изменённым пакетам
    - Один scoped golangci-lint run по изменённым пакетам перед коммитом
created_at: "2026-09-13T21:47:38.658983Z"
updated_at: "2026-09-13T22:00:42.432618Z"
---

## Body

Живой прогон: при превышении заданного контекста (лимит окна сессии) не показывается предупреждение и не отключаются инструменты — сессия продолжает звать тулы, хотя должна была предупредить и уйти в режим без инструментов (к компакции).

**Что ожидается:** при приближении к лимиту — предупреждение (пользователю и/или модели), при превышении — инструменты отключаются.

**Где искать:** подсчёт заполнения контекста, порог компакции (его уже отчитывает тул context), логика скрытия/отключения тулов в executor/сессии.

**Started (2026-09-14).** Диагноз подтверждён красным тестом (контроллер теряет reminder_tokens при смене сессии) + два соучастника: лесенка мерит пост-стаб проекцию, Ollama без usage даёт bytes/4. Скоуп по решению пользователя: проводка + pre-stab давление; оценка токенов — отдельная тема.

**Done (2026-09-14).** Два коммита на bug/context-limit-warning-not-firing (база main 90fdffa8): 80d140cb — EngineOpts.Compaction заново сеет настроенный reminder в свежие двигатели (newEngine — единая точка сборки; Controller.compactionPolicy: override → General → window-derived), красный тест TestClearKeepsConfiguredReminderThreshold позеленел; 692e4f97 — contextStats меряет полный долговременный контекст (BuildContext) вместо пост-стаб проекции (providerContext расщеплён на projectContext), красный тест TestContextStatsCountsFullContextPastStubbing позеленел. Гейты по изменённым пакетам: build/test internal/agent + internal/tui/controller зелёные, один scoped golangci-lint run — 0 issues. CHANGELOG [Unreleased] — обе записи. Не пушено; PR (approver ksilena, подписанные коммиты) ждёт команды. Оценка токенов для не-ASCII (Ollama без usage) — вне скоупа, см. calibrate-context-token-estimate.

## Verification Plan

1. Тест, воспроизводящий несрабатывание, красный до фикса и зелёный после
2. go build + go test по изменённым пакетам
3. Один scoped golangci-lint run по изменённым пакетам перед коммитом
