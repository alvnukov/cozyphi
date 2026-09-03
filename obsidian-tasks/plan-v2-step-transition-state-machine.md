---
id: plan-v2-step-transition-state-machine
title: Expose validated Plan step lifecycle transitions
status: done
priority: high
model_level: low
task_type: feature
parent_id: plan-v2-agent-work-contract
tags:
    - plan-v2
    - state-machine
    - evidence
    - tdd
acceptance_criteria:
    - Разрешённые status transitions проходят, запрещённые возвращают текущий status и допустимые next actions.
    - complete без outcome и evidence/no_evidence_reason отклоняется.
    - block, cancel и reopen без обязательной причины отклоняются.
    - Повтор mutation ID возвращает исходный result без новой revision и duplicate evidence.
    - Completed history не переписывается молча; reopen создаёт аудируемое событие.
verification_plan:
    - Показать red→green transition matrix tests.
    - Запустить session/plantool tests с race detector.
    - Проверить persistence/resume после каждого non-terminal и terminal status.
created_at: "2026-08-28T10:51:18.607556Z"
updated_at: "2026-08-28T14:34:52.543092Z"
---

## Body

Blocked by: plan-v2-atomic-batch-patch. Перевести в todo только после blocker=done.

Добавить semantic actions start, complete, block, resume, cancel и reopen. Status нельзя менять patch-ом. complete требует concise outcome и bounded evidence item/ref либо explicit no_evidence_reason для действительно ненаблюдаемого шага. block требует blocker и resume_when. cancel/reopen требуют reason. Derived checkboxes/UI state строятся только из status. Переходы append audit event и идемпотентны по mutation ID.

TDD seam: public transition API и durable state machine. Один red transition test → minimal green за цикл.

## Acceptance Criteria

- Разрешённые status transitions проходят, запрещённые возвращают текущий status и допустимые next actions.
- complete без outcome и evidence/no_evidence_reason отклоняется.
- block, cancel и reopen без обязательной причины отклоняются.
- Повтор mutation ID возвращает исходный result без новой revision и duplicate evidence.
- Completed history не переписывается молча; reopen создаёт аудируемое событие.

## Verification Plan

1. Показать red→green transition matrix tests.
2. Запустить session/plantool tests с race detector.
3. Проверить persistence/resume после каждого non-terminal и terminal status.
