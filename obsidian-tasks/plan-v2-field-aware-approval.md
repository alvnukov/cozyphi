---
id: plan-v2-field-aware-approval
title: Make Plan approval depend on material contract changes
status: done
priority: high
model_level: low
task_type: feature
parent_id: plan-v2-agent-work-contract
tags:
    - plan-v2
    - approval
    - diff
    - safety
    - tdd
acceptance_criteria:
    - Каждое material изменение детерминированно переводит plan в unapproved и возвращает compact diff.
    - Каждое operational изменение сохраняет текущий approval.
    - Модель не может установить approved через create, patch или transition.
    - Diff показывает add/change/remove по stable IDs без полного plan dump.
    - Material batch и approval change сохраняются атомарно и корректно восстанавливаются после resume.
verification_plan:
    - Показать red→green classification table tests для каждого поля.
    - Запустить focused session/plantool approval tests с race detector.
    - Проверить existing sidebar approval handoff tests на regression.
created_at: "2026-08-28T10:51:18.608183Z"
updated_at: "2026-08-28T15:05:38.51807Z"
---

## Body

Blocked by: plan-v2-atomic-batch-patch, plan-v2-step-transition-state-machine. Перевести в todo после обоих blockers.

Разделить contract fields и operational fields. Goal, approach, success criteria, constraints, step action/type/done_when/risk, add/remove/reorder steps отзывают approval и создают human-readable material diff. Status, outcome, evidence, blocker, working context и wording-only why updates approval не сбрасывают. Approval остаётся user-owned; модель не может выставить его tool call-ом.

TDD seam: session approval policy через публичные update/transition operations.

## Acceptance Criteria

- Каждое material изменение детерминированно переводит plan в unapproved и возвращает compact diff.
- Каждое operational изменение сохраняет текущий approval.
- Модель не может установить approved через create, patch или transition.
- Diff показывает add/change/remove по stable IDs без полного plan dump.
- Material batch и approval change сохраняются атомарно и корректно восстанавливаются после resume.

## Verification Plan

1. Показать red→green classification table tests для каждого поля.
2. Запустить focused session/plantool approval tests с race detector.
3. Проверить existing sidebar approval handoff tests на regression.
