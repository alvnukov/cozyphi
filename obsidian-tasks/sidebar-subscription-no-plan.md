---
id: sidebar-subscription-no-plan
title: 'Sidebar: секция подписки без имени плана'
status: done
priority: medium
task_type: chore
parent_id: sidebar-subscription-status
tags:
    - sidebar
    - usage
acceptance_criteria:
    - Секция subscription в сайдбаре не показывает имя/тип плана — только окна лимитов с барами и временем сброса
    - /usage не меняется; тесты sidebar и sessions зелёные; смержено в main, worktree и ветка удалены
verification_plan:
    - go test -race ./internal/tui/sidebar/... ./internal/tui/sessions/...
    - gofmt -l и один golangci-lint run по изменённым пакетам
    - Коммит, merge --no-ff, chore(tasks), удаление worktree и ветки; без push
created_at: "2026-09-05T21:17:03.597716Z"
updated_at: "2026-09-05T21:18:44.618531Z"
---

## Body

**Что:** пользователь не хочет видеть в сайдбаре тип плана (plus/pro, snake_case идентификаторы из API) — только состояние подписки: бары окон лимитов и время сброса. Строка с PlanName убрана из subscriptionLines; /usage остаётся подробным и не меняется.

**Готово (2026-09-06).** Правка в sidebar.go, тесты sidebar и sessions переписаны на «plus отсутствует», CHANGELOG уточнён. gofmt, go build, go test -race (sidebar, sessions) зелёные; один golangci-lint run — 0 новых замечаний (4 старых в нетронутом коде). Смержено в main --no-ff. Push не делался.

## Acceptance Criteria

- Секция subscription в сайдбаре не показывает имя/тип плана — только окна лимитов с барами и временем сброса
- /usage не меняется; тесты sidebar и sessions зелёные; смержено в main, worktree и ветка удалены

## Verification Plan

1. go test -race ./internal/tui/sidebar/... ./internal/tui/sessions/...
2. gofmt -l и один golangci-lint run по изменённым пакетам
3. Коммит, merge --no-ff, chore(tasks), удаление worktree и ветки; без push
