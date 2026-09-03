---
id: fix-stream-error-stringly
title: 'ошибки стрима строковые: цепочки и errors.Is теряются'
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - TUI/phi run различают cancel/429/auth
    - аккумулятор openai покрыт тестами
verification_plan:
    - go test ./internal/llm/...
created_at: "2026-08-23T15:17:22.112056Z"
updated_at: "2026-09-02T19:01:40.132503Z"
---

## Body

internal/llm/types.go:117-122 StreamEvent.Err string; engine.go:458 переоборачивает fmt.Errorf. errors.Is(err, context.Canceled) и решения по статус-коду (429/авторизация) невозможны downstream. Плюс internal/llm/openai — 0 тестов при самом аккумуляторе стрима. Фикс: typed error (код провайдера + cause), тесты аккумулятора.

## Acceptance Criteria

- TUI/phi run различают cancel/429/auth
- аккумулятор openai покрыт тестами

## Verification Plan

1. go test ./internal/llm/...
