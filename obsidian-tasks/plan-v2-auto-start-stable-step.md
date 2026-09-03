---
id: plan-v2-auto-start-stable-step
title: Auto-start a pending stable Plan step on a valid gated tool call
status: done
priority: high
model_level: low
task_type: feature
parent_id: plan-v2-agent-work-contract
tags:
    - plan-v2
    - plangate
    - executor
    - api-rounds
    - tdd
acceptance_criteria:
    - Валидный tool call по pending stable ID стартует шаг и выполняет tool без отдельного plan call.
    - Invalid schema, missing approval, wrong type или permission denial не меняют status/revision.
    - Runtime tool failure сохраняет step in_progress и доступным для retry.
    - Visible tools включают инструменты eligible pending steps без ослабления type policy.
    - Legacy numeric plan_step продолжает работать до migration task и даёт deprecation signal.
verification_plan:
    - Показать red→green executor/plangate tests для auto-start и failure boundaries.
    - Запустить focused engine/executor/plangate tests с race detector.
    - Измерить, что старт первого шага требует один model tool call вместо plan+tool.
created_at: "2026-08-28T10:51:18.608973Z"
updated_at: "2026-08-28T15:43:39.394044Z"
---

## Body

Blocked by: plan-v2-step-transition-state-machine, plan-v2-field-aware-approval. Перевести в todo после обоих blockers.

Заменить обязательный отдельный start call: gateable tool call с plan_step=<stable ID> может ссылаться на eligible pending step одобренного плана. После schema/permission validation и до dispatch harness атомарно переводит его in_progress. Invalid args не стартуют шаг; runtime failure оставляет его in_progress. Tool visibility учитывает eligible pending steps, иначе модель не увидит tool для первого шага. Numeric 1-based plan_step временно поддержать как legacy input.

TDD seam: executor + plangate public call behavior. Сначала red tests на pending auto-start и no-start при rejected call.

## Acceptance Criteria

- Валидный tool call по pending stable ID стартует шаг и выполняет tool без отдельного plan call.
- Invalid schema, missing approval, wrong type или permission denial не меняют status/revision.
- Runtime tool failure сохраняет step in_progress и доступным для retry.
- Visible tools включают инструменты eligible pending steps без ослабления type policy.
- Legacy numeric plan_step продолжает работать до migration task и даёт deprecation signal.

## Verification Plan

1. Показать red→green executor/plangate tests для auto-start и failure boundaries.
2. Запустить focused engine/executor/plangate tests с race detector.
3. Измерить, что старт первого шага требует один model tool call вместо plan+tool.
