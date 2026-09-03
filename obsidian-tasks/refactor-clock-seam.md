---
id: refactor-clock-seam
title: 'нет шва времени: все тайминги — настенные константы'
status: todo
priority: medium
task_type: refactor
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - тайминги инъектируются, тесты без time.Sleep
verification_plan:
    - go test на debounce/spinner без реальных задержек
created_at: "2026-08-23T15:17:22.116088Z"
updated_at: "2026-08-23T15:17:22.116088Z"
---

## Body

branchPollInterval=1s (editor.go:608), spinnerInterval=1/15s (:612), sphereInterval (pane.go:22), debounce sleep 100ms (pane.go:552), cancel-clear 1200ms (submitter.go:149), bash publish 100ms (bash.go:99), waitOrDone 120ms (controller.go:744). Ни один не инъектируется — тайминговое поведение нетестируемо без реальных снов. Фикс: маленький clock/Timer-модуль в internal/components, инъекция в виджеты.

## Acceptance Criteria

- тайминги инъектируются, тесты без time.Sleep

## Verification Plan

1. go test на debounce/spinner без реальных задержек
