---
id: plan-authoring-telemetry
title: Add bounded privacy-safe plan authoring counters
status: done
model_level: medium
task_type: feature
parent_id: adaptive-durable-plan-authoring
tags:
    - plan-v2
    - telemetry
acceptance_criteria:
    - 'internal/plantel/plantel.go gains counters only: drafts created, approval latency bucket, material reapprovals, patch-retry friction, completion outcome'
    - Counters are bounded, aggregate and carry no plan text, step text, prompts or repository content
    - No new telemetry path influences authoring decisions automatically; output is read-only for humans and dashboards
    - Existing telemetry tests updated and green
    - Docs note the privacy boundary explicitly
verification_plan:
    - go test ./internal/plantel/...
    - 'test: counter values bounded and no free-form fields serialised'
    - 'review check: no call site passes plan or step text into telemetry'
created_at: "2026-08-30T13:22:04.628937Z"
updated_at: "2026-08-30T14:29:14.984984Z"
---

## Body

**Parent:** adaptive-durable-plan-authoring (epic)
**Blocked by:** adaptive-plan-authoring-policy (counters tag outcomes by authoring_policy selector)

**Outcome:** humans can see whether the adaptive grammar reduces plan friction, without any semantic scoring or feedback loop into the model.

**Scope:**
- internal/plantel/plantel.go (bounded counter set only)

**Out of scope:** semantic scoring, automatic policy switching, plan-text capture, archetype retrieval metrics.

## Acceptance Criteria

- internal/plantel/plantel.go gains counters only: drafts created, approval latency bucket, material reapprovals, patch-retry friction, completion outcome
- Counters are bounded, aggregate and carry no plan text, step text, prompts or repository content
- No new telemetry path influences authoring decisions automatically; output is read-only for humans and dashboards
- Existing telemetry tests updated and green
- Docs note the privacy boundary explicitly

## Verification Plan

1. go test ./internal/plantel/...
2. test: counter values bounded and no free-form fields serialised
3. review check: no call site passes plan or step text into telemetry
