---
id: usage-bar-hide-under-1pct
title: 'Usage: окно с расходом <1% не показывать вовсе'
status: done
priority: medium
model_level: medium
task_type: feature
branch: feature/usage-bar-hide-under-1pct
worktree_path: .worktrees/usage-bar-hide-under-1pct
acceptance_criteria:
    - В /usage и sidebar окно с used < 1% не рендерится вообще (нет метки, полоски, значений, reset-строки); при ≥1% окно видно как сейчас
    - Если все окна < 1% — секция/блок квот остаются аккуратными, без сиротских заголовков и пустых строк; фикстуры копируют живую форму payload
    - Scoped тесты/сборка зелёные, один scoped lint, conventional commit, merge --no-ff, ledger chore(tasks), worktree/ветка удалены, без push
verification_plan:
    - 'Красный тест в usagepane и sidebar: окно 0% (числовое) и percent-окно < 1% не содержат █/░; окно ≥ 1% содержит'
    - go test + go build по изменённым пакетам
    - Один scoped golangci-lint run по изменённым пакетам
    - git diff --check, gopls diagnostics чисты
created_at: "2026-09-06T14:14:44.895433Z"
updated_at: "2026-09-06T15:25:14.541071Z"
---

## Body

Пользователь: «нужно чтобы полоски юзадж меньше одного процента не показывались», уточнения: «вообще блок полностью отсутствует», «и текста нет», «если меньше одного процента должно выглядеть как этого лимита просто нет». Итоговое правило: окно юзажа с потрачено < 1% не рендерится вообще — ни метки, ни полоски, ни значений, ни reset-строки, как будто лимита не существует; при ≥ 1% окно выглядит как сейчас. Действует в обоих рендерерах: /usage (usagepane) и sidebar, для числовых и percent-окон. Пограничный случай: ровно 1% — окно видно.

**Почему:** пустые полоски и нулевые значения на окнах без расхода — шум; пользователь хочет, чтобы /usage показывал только реально тратящие лимиты.

Связано с задачей z.ai-monthly-window-humanize-minute-counter (там же научились не выдумывать поля).

**Started (2026-09-06).** План rev 353 апрувнут; работаю по шагу bar-blank-under-1pct.

**Done (2026-09-06).** Implemented and merged in f1960ce / main merge commit: /usage and sidebar omit windows below 1% including labels, values, bars, and per-window reset rows; sidebar omits the whole subscription block when all limits are below 1%. Scoped go test/build and gopls diagnostics passed. The single scoped golangci-lint run reported only the known pre-existing unparam finding in internal/tui/sidebar/sidebar_plan_test.go:74. No push performed.

## Acceptance Criteria

- В /usage и sidebar окно с used < 1% не рендерится вообще (нет метки, полоски, значений, reset-строки); при ≥1% окно видно как сейчас
- Если все окна < 1% — секция/блок квот остаются аккуратными, без сиротских заголовков и пустых строк; фикстуры копируют живую форму payload
- Scoped тесты/сборка зелёные, один scoped lint, conventional commit, merge --no-ff, ledger chore(tasks), worktree/ветка удалены, без push

## Verification Plan

1. Красный тест в usagepane и sidebar: окно 0% (числовое) и percent-окно < 1% не содержат █/░; окно ≥ 1% содержит
2. go test + go build по изменённым пакетам
3. Один scoped golangci-lint run по изменённым пакетам
4. git diff --check, gopls diagnostics чисты
