---
id: refactor-busy-state-predicate
title: 'Busy-состояние: один предикат вместо четырёх источников правды'
status: done
priority: medium
task_type: refactor
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - одно место решает CanSubmit
    - SyncFromSnap удалён
verification_plan:
    - go test ./internal/tui/submit/...
created_at: "2026-08-23T15:17:22.122785Z"
updated_at: "2026-08-24T16:05:41.048215Z"
---

## Body

Занятость выводится четыре раза: snapshot streaming (submitter.go:168), bash running (:171), overlay-предикаты (:180-181), 7-значная Activity-enum лестница (:187-198); ActivityHandler.SyncFromSnap (controller/activity.go:50-81) — примирительный хак между enum и snapshot. Расхождение = тупики 'cannot submit'. Кандидат: один CanSubmit() за Submitter-ом (или маленький stream-state module у bus); Activity остаётся чистой презентацией футера.

## Acceptance Criteria

- одно место решает CanSubmit
- SyncFromSnap удалён

## Verification Plan

1. go test ./internal/tui/submit/...
