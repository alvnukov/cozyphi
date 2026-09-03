---
id: adaptive-plan-authoring-integration
title: Integration gate for adaptive plan authoring scenarios
status: done
model_level: medium
task_type: feature
parent_id: adaptive-durable-plan-authoring
tags:
    - plan-v2
    - integration
    - testing
acceptance_criteria:
    - 'A deterministic scenario suite covers: trivial task, uncertain bug, compound work, read-only run, novel no-match, risky JIT step, custom type names, stale hint, unavailable tool, mid-plan material adaptation'
    - Mid-plan adaptation scenario uses supersede (not cancel) and closes the plan as success
    - Suite runs the real gate/approval/lifecycle path; no mocks of the permission gate
    - docs for adaptive authoring updated; CHANGELOG.md Unreleased section covers the epic's user-visible changes
    - make fmt-check lint test passes
verification_plan:
    - go test ./internal/... (new scenario suite included)
    - make fmt-check lint test
    - manual read-through of docs and CHANGELOG Unreleased entries
created_at: "2026-08-30T13:22:16.094953Z"
updated_at: "2026-08-30T14:51:45.808828Z"
---

## Body

**Parent:** adaptive-durable-plan-authoring (epic)
**Blocked by:** plan-authoring-telemetry and plan-step-supersede (integration needs both branches merged into one picture)

**Outcome:** the whole adaptive authoring increment is proven end-to-end against the real gate, approval, lifecycle and evidence semantics.

**Scope:**
- new integration test package under internal/ (scenario suite)
- docs and CHANGELOG updates

**Out of scope:** new features; behaviour changes belong to earlier tickets.

## Acceptance Criteria

- A deterministic scenario suite covers: trivial task, uncertain bug, compound work, read-only run, novel no-match, risky JIT step, custom type names, stale hint, unavailable tool, mid-plan material adaptation
- Mid-plan adaptation scenario uses supersede (not cancel) and closes the plan as success
- Suite runs the real gate/approval/lifecycle path; no mocks of the permission gate
- docs for adaptive authoring updated; CHANGELOG.md Unreleased section covers the epic's user-visible changes
- make fmt-check lint test passes

## Verification Plan

1. go test ./internal/... (new scenario suite included)
2. make fmt-check lint test
3. manual read-through of docs and CHANGELOG Unreleased entries
