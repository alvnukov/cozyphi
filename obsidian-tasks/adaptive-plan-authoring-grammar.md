---
id: adaptive-plan-authoring-grammar
title: Add bounded adaptive authoring grammar to the plan contract
status: done
model_level: medium
task_type: feature
parent_id: adaptive-durable-plan-authoring
tags:
    - plan-v2
    - authoring
    - prompt
acceptance_criteria:
    - internal/agent/prompt/plan-prompt.tmpl contains a single self-contained authoring grammar block of roughly 130–170 tokens
    - 'Grammar covers: obligations → workstreams → dependencies/uncertainty → evidence boundaries → smallest complete bespoke plan → least sufficient capability type → self-check'
    - Self-check mentions coverage, observability, mergeability and risk as model-side judgement only, not a harness validator
    - No mention of archetypes, role enum, hidden Model/Actions fields, or tool-schema duplication
    - go test ./internal/... passes with the changed template
verification_plan:
    - go test ./internal/agent/prompt/... ./internal/tools/plantool/...
    - 'grep plan-prompt.tmpl for forbidden tokens: archetype, role enum, Model, Actions'
    - token-count check of the grammar block via the new test
created_at: "2026-08-30T13:21:27.809032Z"
updated_at: "2026-08-30T13:41:52.331422Z"
---

## Body

**Parent:** adaptive-durable-plan-authoring (epic)

**Outcome:** the model-facing plan contract teaches plan shape, not just type permissions.

**Scope:**
- internal/agent/prompt/plan-prompt.tmpl (authoring guidance section)
- compile-time check that the block stays within the token budget (test)

**Out of scope:** plan.defaults changes, archetypes, telemetry, lifecycle changes.

**Adaptive policy:** this ticket is the first increment of the `adaptive-minimal` authoring policy; no selector yet.

## Acceptance Criteria

- internal/agent/prompt/plan-prompt.tmpl contains a single self-contained authoring grammar block of roughly 130–170 tokens
- Grammar covers: obligations → workstreams → dependencies/uncertainty → evidence boundaries → smallest complete bespoke plan → least sufficient capability type → self-check
- Self-check mentions coverage, observability, mergeability and risk as model-side judgement only, not a harness validator
- No mention of archetypes, role enum, hidden Model/Actions fields, or tool-schema duplication
- go test ./internal/... passes with the changed template

## Verification Plan

1. go test ./internal/agent/prompt/... ./internal/tools/plantool/...
2. grep plan-prompt.tmpl for forbidden tokens: archetype, role enum, Model, Actions
3. token-count check of the grammar block via the new test
