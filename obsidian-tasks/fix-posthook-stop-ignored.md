---
id: fix-posthook-stop-ignored
title: post-hook Stop вычисляется и выбрасывается
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - post-hook Stop останавливает агентный цикл с Reason
    - тест exit-2 останавливает цикл
verification_plan:
    - go test ./internal/agent/... ./internal/hooks/...
created_at: "2026-08-23T15:17:22.111214Z"
updated_at: "2026-09-02T19:01:40.131924Z"
---

## Body

internal/agent/executor.go:190 — комментарий 'post.Stop is ignored until a later slice wires it'; hooks.Manager.PostTool агрегирует Stop/Reason (hooks/manager.go:308-313), CommandHook exit-2 обещает hard-deny, но executor это отбрасывает. Аудит-хук не может остановить цикл. Полусобранный шов: интерфейс обещает поведение, которого вызывающий не получает.

## Acceptance Criteria

- post-hook Stop останавливает агентный цикл с Reason
- тест exit-2 останавливает цикл

## Verification Plan

1. go test ./internal/agent/... ./internal/hooks/...
