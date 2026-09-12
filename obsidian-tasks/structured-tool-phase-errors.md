---
id: structured-tool-phase-errors
title: Структурированный отказ вместо тихого скрытия инструментов в фазах плана
status: in_progress
priority: medium
model_level: high
task_type: feature
tags:
    - plangate
    - tools
    - useplan
    - prompt
acceptance_criteria:
    - Схемы инструментов остаются видимыми при смене фазы, статуса шага и режима плана; исполнение по-прежнему ограничено гейтом и отсутствием хендлеров.
    - Заблокированный вызов возвращает модели JSON tool_error с кодом, текущей фазой, причиной, следующим действием и явным запретом обхода через другой инструмент, shell или делегирование.
    - Транскрипт сохраняет прежнюю текстовую причину, совместимость с approval-resume не ломается.
    - Регрессии покрывают plan-режим, useplan/deny, несогласованный план и несовместимый шаг.
verification_plan:
    - Точечные тесты ./internal/agent/... и ./internal/plangate/...
    - happ code op=diagnostics по изменённым файлам
    - Один scoped golangci-lint по изменённым пакетам
    - Ревью diff по шести осям качества AGENTS.md
    - Запись в CHANGELOG под Unreleased и подписанный коммит в ветке worktree без push
created_at: "2026-09-12T20:21:38.57205Z"
updated_at: "2026-09-12T20:21:38.57205Z"
---

## Body

Отменяет тихое скрытие схем, введённое в feat-plan-step-tool-visibility (done, 2026-08-27). Silent removal оставляет модели слишком много пространства интерпретации: слабая модель после исчезновения инструмента начинает хаотично дёргать другие или эмулировать действие через shell. Вместо этого каталог инструментов остаётся стабильным, а отказ фазы приходит как структурированная ошибка с единственным допустимым следующим переходом.

**Origin (2026-09-12).** Реализация написана сессией Codex CLI (rollout-2026-09-12T11-50-24) по запросу из ChatGPT-переписки; в реестре задача заведена постфактум, изменения остались незакоммиченными в рабочем дереве main.

**Scope.** doc/plan-authoring.md, internal/agent/{engine.go,executor.go,tool_unavailable.go,prompt/plan-prompt.tmpl}, internal/plangate/{plangate.go,policy.go}, тесты engine_plan_visibility_test.go, executor_plan_recovery_test.go, executor_unavailable_test.go, CHANGELOG.md.

**Out of scope.** Изменение go.sum (посторонняя правка в main) и obsidian-tasks/user-plan-edits-priority.md.

**Открыто.** Эффект на поведение реальных слабых моделей не измерен.

## Acceptance Criteria

- Схемы инструментов остаются видимыми при смене фазы, статуса шага и режима плана; исполнение по-прежнему ограничено гейтом и отсутствием хендлеров.
- Заблокированный вызов возвращает модели JSON tool_error с кодом, текущей фазой, причиной, следующим действием и явным запретом обхода через другой инструмент, shell или делегирование.
- Транскрипт сохраняет прежнюю текстовую причину, совместимость с approval-resume не ломается.
- Регрессии покрывают plan-режим, useplan/deny, несогласованный план и несовместимый шаг.

## Verification Plan

1. Точечные тесты ./internal/agent/... и ./internal/plangate/...
2. happ code op=diagnostics по изменённым файлам
3. Один scoped golangci-lint по изменённым пакетам
4. Ревью diff по шести осям качества AGENTS.md
5. Запись в CHANGELOG под Unreleased и подписанный коммит в ветке worktree без push
