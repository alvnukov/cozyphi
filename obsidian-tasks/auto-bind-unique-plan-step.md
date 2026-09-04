---
id: auto-bind-unique-plan-step
title: Автоматически привязывать однозначный plan step
status: todo
priority: high
model_level: medium
task_type: feature
parent_id: reliable-model-file-edits
acceptance_criteria:
    - При неверном, отсутствующем или завершённом plan_step и ровно одном совместимом active/pending шаге вызов автоматически привязывается к нему
    - Автопривязка проходит через существующий atomic auto-start и сохраняет step type, skill preload и evidence semantics
    - При нескольких кандидатах вызов отклоняется и выводит bounded-список допустимых id, status и type
    - Числовой plan_step диагностируется как deprecated и автоматически исправляется только при единственном кандидате
    - Permission gate и требования approval/JIT не ослаблены
verification_plan:
    - 'Добавить table-driven policy-тесты: exact id, missing/invalid/numeric/completed id, one/many/no candidates'
    - Добавить интеграционные тесты atomic pending auto-start и skill preload
    - Проверить, что unapproved и JIT планы остаются заблокированы
    - Запустить go test -race для plangate и session orchestration
created_at: "2026-09-04T22:22:01.721865Z"
updated_at: "2026-09-04T22:22:01.721865Z"
---

## Body

Убрать ошибки bookkeeping вокруг plan_step, когда намерение однозначно. Plan binding не является filesystem/security permission: если существует ровно один шаг, который может принять вызов, harness должен выбрать его сам. При реальной неоднозначности он не угадывает, а показывает конкретные варианты.

Auto-binding должен быть частью policy/seam план-гейта, а не исправляться отдельно каждым tool caller. Нельзя обходить approval, JIT, tool-rank или skill-preload choreography.

**Blocked by:** None — can start immediately. Независимая архитектурная линия, может выполняться параллельно edit-задачам.

## Acceptance Criteria

- При неверном, отсутствующем или завершённом plan_step и ровно одном совместимом active/pending шаге вызов автоматически привязывается к нему
- Автопривязка проходит через существующий atomic auto-start и сохраняет step type, skill preload и evidence semantics
- При нескольких кандидатах вызов отклоняется и выводит bounded-список допустимых id, status и type
- Числовой plan_step диагностируется как deprecated и автоматически исправляется только при единственном кандидате
- Permission gate и требования approval/JIT не ослаблены

## Verification Plan

1. Добавить table-driven policy-тесты: exact id, missing/invalid/numeric/completed id, one/many/no candidates
2. Добавить интеграционные тесты atomic pending auto-start и skill preload
3. Проверить, что unapproved и JIT планы остаются заблокированы
4. Запустить go test -race для plangate и session orchestration
