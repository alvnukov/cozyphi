---
id: refactor-posthook-stop-typed-signal
title: PostTool stop передаётся строкой и сравнивается с литералом
status: done
priority: low
task_type: refactor
parent_id: fix-posthook-stop-ignored
tags:
    - hooks
    - agent
    - review-2026-09
acceptance_criteria:
    - Причина stop и её отсутствие выражены типом, а не сравнением строк
    - Литерал причины по умолчанию встречается один раз
verification_plan:
    - go test ./internal/agent/ ./internal/hooks/
created_at: "2026-09-02T16:51:43.323765Z"
updated_at: "2026-09-02T19:55:56.682416Z"
---

## Body

**Проблема.** Сигнал stop живёт как `string`, где пустая строка значит «стопа нет»: `internal/agent/executor.go:441` пишет литерал «post-tool hook requested stop», а `internal/agent/engine.go:889` сравнивает с тем же литералом, чтобы понять, была ли причина. `runOne(ctx, call, emit, skillRetryRequired *bool, stopReason *string)` (`executor.go:260-266`) тащит два out-указателя, ради тройного возврата `run` правились десять тестовых файлов.

**Как чинить.** Возвращать `error` (уже есть `ErrPostHookStop`) или `hookStop{Reason string}`; литерал живёт в одном месте; два out-указателя собрать в структуру состояния раунда. Найдено ревью правок после v0.19.0.

## Acceptance Criteria

- Причина stop и её отсутствие выражены типом, а не сравнением строк
- Литерал причины по умолчанию встречается один раз

## Verification Plan

1. go test ./internal/agent/ ./internal/hooks/
