---
id: plan-step-supersede
title: Add audited supersede for plan step capability changes
status: done
model_level: medium
task_type: feature
parent_id: adaptive-durable-plan-authoring
tags:
    - plan-v2
    - lifecycle
acceptance_criteria:
    - plan patch API gains an audited atomic supersede operation for a single step
    - Old step keeps its evidence/audit trail; replacement gets a fresh stable id and capability type
    - Superseding a completed, in_progress or approved-material step triggers reapproval exactly like other material plan changes
    - Superseded plans close as success, fixing the cancel-then-close trap in internal/session/plan_transition.go
    - Only pending steps can be removed (existing rule in internal/session/plan_patch.go stays intact)
    - go test ./internal/session/... passes with new supersede coverage
verification_plan:
    - go test ./internal/session/...
    - 'new test: supersede preserves prior step evidence in audit history'
    - 'new test: plan with a superseded step closes as success'
    - 'new test: non-material supersede does not force reapproval; material one does'
created_at: "2026-08-30T13:21:52.543497Z"
updated_at: "2026-08-30T14:04:57.257471Z"
---

## Body

**Parent:** adaptive-durable-plan-authoring (epic)

**Outcome:** mid-plan adaptation can change a step's capability type without rewriting completed evidence and without poisoning plan completion.

**Scope:**
- internal/session/plan_patch.go (new supersede op alongside existing ops)
- internal/session/plan_transition.go (completion accounting for superseded steps)
- internal/session/plan_diff.go (material-change detection triggers reapproval)

**Out of scope:** update_step retyping, archetype retrieval, telemetry, prompt changes.

**Design note:** supersede replaces the current cancel-based workaround; cancellation semantics themselves stay untouched.

## Acceptance Criteria

- plan patch API gains an audited atomic supersede operation for a single step
- Old step keeps its evidence/audit trail; replacement gets a fresh stable id and capability type
- Superseding a completed, in_progress or approved-material step triggers reapproval exactly like other material plan changes
- Superseded plans close as success, fixing the cancel-then-close trap in internal/session/plan_transition.go
- Only pending steps can be removed (existing rule in internal/session/plan_patch.go stays intact)
- go test ./internal/session/... passes with new supersede coverage

## Verification Plan

1. go test ./internal/session/...
2. new test: supersede preserves prior step evidence in audit history
3. new test: plan with a superseded step closes as success
4. new test: non-material supersede does not force reapproval; material one does
