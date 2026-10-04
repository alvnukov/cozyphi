---
id: codex-models-client-version-0.160
title: Синхронизировать каталог моделей с Codex 0.160.0
status: done
priority: high
model_level: low
task_type: bug
tags:
    - provider
    - codex
branch: bug/codex-models-client-version-0.160
worktree_path: .worktrees/codex-models-client-version-0.160
acceptance_criteria:
    - Codex /models request sends client_version=0.160.0
    - A cache written by 0.156.0 is refreshed
    - go test ./internal/provider passes
verification_plan:
    - go test ./internal/provider
    - golangci-lint run ./internal/provider
created_at: "2026-10-04T23:35:20.367319Z"
updated_at: "2026-10-04T23:39:22.037633Z"
---

## Body

Синхронизировать версию клиента каталога моделей с официальным @openai/codex 0.160.0. Не добавлять модели вручную; backend сам возвращает доступный аккаунту список.

**Started (2026-10-05).** Официальный npm latest и tag rust-v0.160.0 подтверждают текущую версию 0.160.0.

**Done (2026-10-05).** Обновлён codexModelsClientVersion 0.156.0→0.160.0; запрос проверяется точным тестом, кэш 0.156.0 инвалидируется. go test ./internal/provider и git diff --check прошли. Scoped golangci-lint не выполнился: установленный lint собран Go 1.26, а исходники требуют Go 1.27.

## Acceptance Criteria

- Codex /models request sends client_version=0.160.0
- A cache written by 0.156.0 is refreshed
- go test ./internal/provider passes

## Verification Plan

1. go test ./internal/provider
2. golangci-lint run ./internal/provider
