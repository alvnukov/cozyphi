---
id: plan-v2-piggyback-transition
title: Piggyback Plan transition metadata on the next working tool call
status: done
priority: high
model_level: low
task_type: feature
parent_id: plan-v2-agent-work-contract
tags:
    - plan-v2
    - executor
    - api-rounds
    - atomicity
    - tdd
acceptance_criteria:
    - Один tool call атомарно settles previous step, updates context и auto-starts target before dispatch.
    - Invalid _plan или invalid target tool args не оставляют partial plan mutation и не dispatch-ят tool.
    - Runtime failure target tool не откатывает уже подтверждённый previous outcome и сохраняет target in_progress.
    - _plan удаляется до strict decode целевого инструмента.
    - Retry/reconciliation одного call ID не дублирует completion, context revision или attempt.
verification_plan:
    - Показать red→green executor integration tests happy path и трёх failure boundaries.
    - Запустить executor/plangate/session tests с race detector.
    - 'Подтвердить trace: между двумя рабочими шагами нет отдельного plan-only model round.'
created_at: "2026-08-28T10:51:18.610692Z"
updated_at: "2026-08-28T22:06:21.511797Z"
---

## Body

Blocked by: plan-v2-atomic-batch-patch, plan-v2-step-transition-state-machine, plan-v2-auto-start-stable-step, plan-v2-tool-attempt-evidence. Перевести в todo после всех blockers.

Добавить общий harness-owned _plan envelope, который strict tool schemas не получают. В одном следующем working call модель может завершить предыдущий step с outcome/evidence, заменить working context и активировать/использовать текущий stable step. Сначала валидируются plan mutation, target step и domain tool args; invalid plan metadata не dispatch-ит domain tool. После успешной валидации completion предыдущего шага сохраняется независимо; runtime failure нового tool оставляет новый step active. Mutation идемпотентна по tool-call ID.

TDD seam: executor dispatch boundary.

## Acceptance Criteria

- Один tool call атомарно settles previous step, updates context и auto-starts target before dispatch.
- Invalid _plan или invalid target tool args не оставляют partial plan mutation и не dispatch-ят tool.
- Runtime failure target tool не откатывает уже подтверждённый previous outcome и сохраняет target in_progress.
- _plan удаляется до strict decode целевого инструмента.
- Retry/reconciliation одного call ID не дублирует completion, context revision или attempt.

## Verification Plan

1. Показать red→green executor integration tests happy path и трёх failure boundaries.
2. Запустить executor/plangate/session tests с race detector.
3. Подтвердить trace: между двумя рабочими шагами нет отдельного plan-only model round.
