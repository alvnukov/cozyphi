---
id: validate-tool-args
title: Validate every tool call against its schema before the gates
status: done
priority: high
task_type: bug
tags:
    - harness
    - tools
    - executor
acceptance_criteria:
    - An unknown or mistyped argument is refused before the permission gate with the declared keys named
    - A missing plan_step on a gated tool still gets the plan gate's own verdict
    - edit's file_path alias still works end to end
    - go test ./internal/agent/... ./internal/tools/... green; scoped lint clean
    - 'CI on PR #14 green across the whole repository'
verification_plan:
    - go test ./internal/agent/... ./internal/tools/... ./internal/session/... ./internal/plangate/...
    - go test ./internal/tui/controller/ -run TestPlanAutoApproveSurvivesClearAndResume
    - golangci-lint run ./internal/agent/ ./internal/tools/tooldef/
created_at: "2026-09-09T21:39:00.600959Z"
updated_at: "2026-09-09T21:51:13.561019Z"
---

## Body

**Problem.** The executor validated tool arguments against the declared JSON schema only for calls carrying a `_plan` envelope. A plain call with an unknown or mistyped argument passed the plan gate and the permission gate, could prompt the user, and failed only inside the tool with whatever message its decoder produced (lenient decoders silently dropped the key).

**Change.** `tooldef.ValidateAgainstSchema` runs in `Executor.runOne` on every call after PreTool hooks and before the plan gate. The validator names the declared keys on an unknown argument, leaves `plan_step` to the plan gate even when the injected schema requires it, accepts the `file_path` alias for a declared `path`, and leaves a schema-less tool to its own decoder. The duplicate check in `settleOrStart` is gone.

**Fallout.** `agent_spawn` no longer reaches its tailored `model` refusal through the executor: the generic unknown-argument refusal lists `effort` instead. Test fixtures that declared an object schema without properties and then sent keys now declare those keys; legacy plan fixtures without `action` send `action: update`. The first CI run of PR #14 caught one more such fixture outside the scoped packages: the TUI controller's auto-approve test drove `plan` with only `steps`, which the validator now refuses before the tool; it sends `action: update` too. Fixtures that call `plan.Run` directly never pass the executor and stay as they are.

## Acceptance Criteria

- An unknown or mistyped argument is refused before the permission gate with the declared keys named
- A missing plan_step on a gated tool still gets the plan gate's own verdict
- edit's file_path alias still works end to end
- go test ./internal/agent/... ./internal/tools/... green; scoped lint clean
- CI on PR #14 green across the whole repository

## Verification Plan

1. go test ./internal/agent/... ./internal/tools/... ./internal/session/... ./internal/plangate/...
2. go test ./internal/tui/controller/ -run TestPlanAutoApproveSurvivesClearAndResume
3. golangci-lint run ./internal/agent/ ./internal/tools/tooldef/
