---
id: plan-v2-create-get-actions
title: Add create/get Plan v2 tool actions with compact views
status: done
priority: high
model_level: low
task_type: feature
parent_id: plan-v2-agent-work-contract
tags:
    - plan-v2
    - tool-api
    - context
    - tdd
acceptance_criteria:
    - create создаёт durable unapproved v2 plan и возвращает короткий revision/progress result.
    - get active возвращает goal, critical constraints, active/next summary и blockers в пределах budget.
    - get full возвращает полный canonical snapshot только по явному запросу.
    - Unknown fields/actions и incomplete v2 contract дают детерминированные actionable errors.
    - Legacy steps-only вызов остаётся совместимым и помечается compatibility metadata.
verification_plan:
    - Показать red→green table tests публичного plan tool JSON contract.
    - Запустить focused tests plantool и session.
    - Проверить размер compact и full responses отдельными assertions.
created_at: "2026-08-28T10:51:18.605721Z"
updated_at: "2026-08-28T13:16:06.423709Z"
---

## Body

Blocked by: plan-v2-expand-durable-contract. Перевести в todo только после blocker=done.

Добавить discriminated plan actions create и get поверх canonical v2 model. create принимает полный контракт и создаёт unapproved draft. get поддерживает bounded active/default view и явный full view; обычный ответ компактный. Legacy steps-only update продолжает работать в compatibility path. Ошибки должны называть отсутствующее поле и допустимое действие.

TDD seam: публичный plan tool JSON contract. Сначала red tests на create, compact get, full get, strict decoding и legacy compatibility.

## Acceptance Criteria

- create создаёт durable unapproved v2 plan и возвращает короткий revision/progress result.
- get active возвращает goal, critical constraints, active/next summary и blockers в пределах budget.
- get full возвращает полный canonical snapshot только по явному запросу.
- Unknown fields/actions и incomplete v2 contract дают детерминированные actionable errors.
- Legacy steps-only вызов остаётся совместимым и помечается compatibility metadata.

## Verification Plan

1. Показать red→green table tests публичного plan tool JSON contract.
2. Запустить focused tests plantool и session.
3. Проверить размер compact и full responses отдельными assertions.
