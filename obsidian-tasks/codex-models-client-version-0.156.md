---
id: codex-models-client-version-0.156
title: Бамп codexModelsClientVersion до 0.156.0 — новые модели подписки скрыты бэкендом
status: in_progress
priority: high
task_type: bug
tags:
    - provider
    - codex
branch: bug/codex-models-client-version-0.156
worktree_path: .worktrees/codex-models-client-version-0.156
acceptance_criteria:
    - gpt-6-sol и gpt-6-luna появляются в каталоге после обновления (live-проверка client_version=0.156.0 возвращает обе)
    - свежий кэш прежней версии не блокирует обновление (существующий тест)
    - go test ./internal/provider проходит
verification_plan:
    - go test ./internal/provider
    - golangci-lint run ./internal/provider
    - живой повторный RefreshSubscriptionModels показывает gpt-6-sol/gpt-6-luna
created_at: "2026-09-22T22:17:56.613015Z"
updated_at: "2026-09-22T22:18:01.889334Z"
---

## Body

Диагноз (2026-09-22, живой запрос с аккаунта подписки): GET chatgpt.com/backend-api/codex/models?client_version=0.153.1 возвращает 7 моделей без gpt-6-sol/gpt-6-luna (обе minimal_client_version=0.155.0); client_version=0.156.0 возвращает обе. cozyphi шлёт пиннутый codexModelsClientVersion="0.153.1" (internal/provider/codex_models.go), бэкенд фильтрует модели по минимальной версии клиента — как codex (models-manager client_version_to_whole = версия CLI). Фикс: бамп константы до текущего релиза @openai/codex 0.156.0 (проверено живым ответом: поля slug/display_name/visibility/priority/context_window на месте). Прецедент: fix-codex-astra-discovery (бамп 0.145.0→0.153.1). Секреты в задачу не писать.

## Acceptance Criteria

- gpt-6-sol и gpt-6-luna появляются в каталоге после обновления (live-проверка client_version=0.156.0 возвращает обе)
- свежий кэш прежней версии не блокирует обновление (существующий тест)
- go test ./internal/provider проходит

## Verification Plan

1. go test ./internal/provider
2. golangci-lint run ./internal/provider
3. живой повторный RefreshSubscriptionModels показывает gpt-6-sol/gpt-6-luna

## Result

PR #54: бамп codexModelsClientVersion 0.153.1→0.156.0, живая проверка (тот же аккаунт/токен): 0.153.1 → 7 моделей без gpt-6-sol/gpt-6-luna; 0.156.0 → 9. go test ./internal/provider и golangci-lint run ./internal/provider — чисто.
