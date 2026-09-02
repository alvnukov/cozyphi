---
id: plan-v2-expand-durable-contract
title: Expand durable Plan schema with v2 contract fields and legacy decoding
status: done
priority: high
model_level: low
task_type: refactor
parent_id: plan-v2-agent-work-contract
tags:
    - plan-v2
    - persistence
    - schema
    - expand-contract
    - tdd
acceptance_criteria:
    - V2 plan и step fields сохраняются и восстанавливаются без потерь через public session plan seam.
    - Legacy persisted plan с content и numeric ordering загружается в canonical runtime shape без panic и data loss.
    - V2 creation rejects missing goal/approach/success criteria and missing step id/action/why/done_when; legacy load остаётся допустимым.
    - Каждое текстовое поле и весь serialized plan имеют явные проверенные bounds.
    - Legacy full-snapshot plan tool contract и существующие tests остаются green.
verification_plan:
    - Показать red→green focused tests для session plan round-trip, legacy decode и bounds.
    - Запустить все tests пакета session с race detector.
    - Запустить существующие focused plan tool tests для проверки совместимости.
created_at: "2026-08-28T10:51:18.604679Z"
updated_at: "2026-08-28T12:48:18.69454Z"
---

## Body

Blocked by: none — frontier task.

Сделать expand-фазу без переключения tool API. В durable Plan добавить goal, approach, success criteria, constraints, bounded working context и lifecycle/result metadata. В step добавить stable string ID, canonical action, required-for-v2 why/done_when, outcome, optional risk/JIT metadata и место для bounded evidence refs. Старые persisted sessions и старый content-based snapshot обязаны загружаться в один canonical runtime representation; не держать две истины. Legacy tool update пока продолжает работать.

TDD seam: session plan persistence/round-trip. Сначала тесты, которые падают на v2 round-trip, legacy decode и bounds; затем минимальная реализация. Не менять gate, prompt и sidebar в этой задаче.

## Acceptance Criteria

- V2 plan и step fields сохраняются и восстанавливаются без потерь через public session plan seam.
- Legacy persisted plan с content и numeric ordering загружается в canonical runtime shape без panic и data loss.
- V2 creation rejects missing goal/approach/success criteria and missing step id/action/why/done_when; legacy load остаётся допустимым.
- Каждое текстовое поле и весь serialized plan имеют явные проверенные bounds.
- Legacy full-snapshot plan tool contract и существующие tests остаются green.

## Verification Plan

1. Показать red→green focused tests для session plan round-trip, legacy decode и bounds.
2. Запустить все tests пакета session с race detector.
3. Запустить существующие focused plan tool tests для проверки совместимости.
