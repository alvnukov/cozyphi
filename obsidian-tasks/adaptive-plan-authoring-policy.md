---
id: adaptive-plan-authoring-policy
title: Wire closed authoring_policy selector through plan.defaults
status: done
model_level: medium
task_type: feature
parent_id: adaptive-durable-plan-authoring
tags:
    - plan-v2
    - authoring
    - config
acceptance_criteria:
    - plan.defaults gains a closed selector authoring_policy with allowed values adaptive-minimal and legacy; arbitrary text/templates rejected at load
    - internal/config/plan.go and internal/harnesssettings/draft.go carry the selector end-to-end, including settings UI exposure
    - internal/agent/prompt/plan-prompt.tmpl selects the grammar block only when adaptive-minimal is active; legacy prompt unchanged
    - Selector changes no permissions, gate, approval or lifecycle behaviour (existing plangate tests pass unchanged)
    - Docs for the selector added and CHANGELOG.md gains an Unreleased line
verification_plan:
    - go test ./internal/config/... ./internal/harnesssettings/... ./internal/agent/prompt/...
    - 'config load test: invalid authoring_policy value is rejected'
    - plangate regression tests unchanged and green
created_at: "2026-08-30T13:21:40.008972Z"
updated_at: "2026-08-30T14:21:35.823622Z"
---

## Body

**Parent:** adaptive-durable-plan-authoring (epic)
**Blocked by:** adaptive-plan-authoring-grammar (the block it selects must exist first)

**Outcome:** users can opt into the adaptive grammar via one closed, validated selector; nothing free-form enters config.

**Scope:**
- internal/config/plan.go
- internal/harnesssettings/draft.go
- internal/agent/prompt/plan-prompt.tmpl (selection, not content)
- internal/components/settings/plan_form.go, internal/tui/settings/pane.go

**Out of scope:** new capability types, archetype catalog, telemetry, lifecycle semantics.

**Enforcement boundary:** plan.defaults stays enforcement config; the selector is a closed enum, never an instruction channel.

## Acceptance Criteria

- plan.defaults gains a closed selector authoring_policy with allowed values adaptive-minimal and legacy; arbitrary text/templates rejected at load
- internal/config/plan.go and internal/harnesssettings/draft.go carry the selector end-to-end, including settings UI exposure
- internal/agent/prompt/plan-prompt.tmpl selects the grammar block only when adaptive-minimal is active; legacy prompt unchanged
- Selector changes no permissions, gate, approval or lifecycle behaviour (existing plangate tests pass unchanged)
- Docs for the selector added and CHANGELOG.md gains an Unreleased line

## Verification Plan

1. go test ./internal/config/... ./internal/harnesssettings/... ./internal/agent/prompt/...
2. config load test: invalid authoring_policy value is rejected
3. plangate regression tests unchanged and green
