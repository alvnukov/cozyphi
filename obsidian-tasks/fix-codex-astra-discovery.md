---
id: fix-codex-astra-discovery
title: Вернуть Astra в каталог моделей подписки Codex
status: done
priority: high
task_type: bug
branch: codex/fix-codex-astra-discovery
worktree_path: .worktrees/fix-codex-astra-discovery
acceptance_criteria:
    - Astra доступна после обновления каталога
    - Свежий кэш прежней версии не блокирует обновление
verification_plan:
    - Регрессионный тест RefreshSubscriptionModels
    - go test ./internal/provider
    - golangci-lint run ./internal/provider
created_at: "2026-09-05T09:51:43.753893Z"
updated_at: "2026-09-05T09:54:32.011079Z"
---

## Body

Подтверждено запросами с одной учётной записью: client_version=0.145.0 не возвращает gpt-6-astra, 0.153.1 возвращает visibility=list. Обновить версию совместимости, проверить замену свежего кэша предыдущей версии и сохранение Astra. Секреты не записывать.

## Acceptance Criteria

- Astra доступна после обновления каталога
- Свежий кэш прежней версии не блокирует обновление

## Verification Plan

1. Регрессионный тест RefreshSubscriptionModels
2. go test ./internal/provider
3. golangci-lint run ./internal/provider
