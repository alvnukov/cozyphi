---
id: refactor-engine-reconfigure
title: 'Engine: атомарный reconfigure вместо caller-remembered dance из 6 шагов'
status: todo
priority: high
task_type: refactor
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - порядок инициализации знает только Engine
    - readonly-движок переживает reconfigure
    - одна пересборка на смену модели
verification_plan:
    - go test ./internal/agent/... ./internal/tui/controller/...
created_at: "2026-08-23T15:17:22.116821Z"
updated_at: "2026-08-23T15:17:22.116821Z"
---

## Body

Движок — mutable god object: клиент, executor, гейт, ask, jobs, hooks, mcp, компакция, пересборка через 7 сеттеров. Контроллер обязан звать Cancel -> initGate -> SetPermission -> SetContinueAsk -> SetJobs -> ReloadHooks -> SetModel по порядку (controller.go:352-387); SetJobs и SetModel каждый пересобирает executor+client+prompt (двойная пересборка на смену модели); SetPermission тычет engine.executor.gate напрямую (:209-213). Кандидат: хранить базовый список тулов и один reconfigure(model, gate, ask, jobs, hooks), пересобирающий всё атомарно. Поглощает fix-rebind-tools-resets-readonly и знание порядка из контроллера. Deletion test: удаление любого из пяти методов пересборки только двигает баг — граница module не там.

## Acceptance Criteria

- порядок инициализации знает только Engine
- readonly-движок переживает reconfigure
- одна пересборка на смену модели

## Verification Plan

1. go test ./internal/agent/... ./internal/tui/controller/...
