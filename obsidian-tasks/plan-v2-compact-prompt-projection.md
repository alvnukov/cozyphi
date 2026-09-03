---
id: plan-v2-compact-prompt-projection
title: Inject a bounded, decision-rich Plan projection into model context
status: done
priority: high
model_level: low
task_type: feature
parent_id: plan-v2-agent-work-contract
tags:
    - plan-v2
    - prompt
    - tokens
    - context
    - tdd
acceptance_criteria:
    - Short plan projection содержит всю decision-critical информацию без full audit.
    - Max-size plan остаётся в зафиксированном total budget и обрезается в документированном priority order.
    - Active why/done_when и critical constraints не теряются раньше completed evidence/history.
    - Completed steps схлопываются до ID+outcome; raw tool logs не инжектируются.
    - Projection явно маркирует plan fields как untrusted/model-authored data.
verification_plan:
    - Показать red→green golden/table tests short, blocked и max-size projections.
    - Измерить serialized bytes для 1, 8 и 32 steps и закрепить upper bound.
    - Запустить prompt/engine tests и prompt-injection fixtures.
created_at: "2026-08-28T10:51:18.611678Z"
updated_at: "2026-08-28T16:51:00.88336Z"
---

## Body

Blocked by: plan-v2-create-get-actions, plan-v2-step-transition-state-machine, plan-v2-tool-attempt-evidence. Перевести в todo после всех blockers.

Разделить durable storage и always-injected projection. Всегда показывать goal, approach, critical constraints, progress, active step полностью (action/why/done_when), unresolved blockers/context, краткие completed outcomes и ближайшие steps. Старые evidence logs, audit и full snapshot доступны только get full. Ввести общий byte/token-oriented budget и детерминированное truncation priority. Model-authored text оборачивается как data, не как system instructions.

TDD seam: prompt projection public renderer.

## Acceptance Criteria

- Short plan projection содержит всю decision-critical информацию без full audit.
- Max-size plan остаётся в зафиксированном total budget и обрезается в документированном priority order.
- Active why/done_when и critical constraints не теряются раньше completed evidence/history.
- Completed steps схлопываются до ID+outcome; raw tool logs не инжектируются.
- Projection явно маркирует plan fields как untrusted/model-authored data.

## Verification Plan

1. Показать red→green golden/table tests short, blocked и max-size projections.
2. Измерить serialized bytes для 1, 8 и 32 steps и закрепить upper bound.
3. Запустить prompt/engine tests и prompt-injection fixtures.
