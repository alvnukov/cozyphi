---
id: web-plan-step-strict-decode
title: Make the web tool accept the plan_step the plan gate requires
status: done
priority: high
model_level: medium
task_type: bug
tags:
    - tools
    - plangate
    - web
acceptance_criteria:
    - A web call carrying plan_step decodes instead of failing with unknown field plan_step.
    - A regression test proves every plan-gated tool accepts plan_step in its arguments.
    - CHANGELOG records the fix under Unreleased.
verification_plan:
    - go test ./internal/tools/ ./internal/tools/webtool/
    - scoped golangci-lint run over internal/tools/...
created_at: "2026-09-09T21:17:54.369659Z"
updated_at: "2026-09-09T21:19:45.262218Z"
---

## Body

**Symptom.** The plan gate injects `plan_step` into the schema of every non-exempt tool and marks it required. `web` decodes its arguments with `tooldef.DecodeStrict`, and its args struct reserved no `plan_step` field, so the call the model was told to make fails: `web: json: unknown field "plan_step"`. Reproduced on the real constructor with `{"action":"search","query":"x","plan_step":"s1"}`; the same call without plan_step passes decoding.

**Cause.** lsp, memory, watch, task and harness reserve the name with the `tooldef.PlanStep` sentinel; web was added without it. Found while comparing tool descriptions against a Claude Code session log.

**Fix.** Reserve `plan_step` in the web args struct and add a contract test in internal/tools that runs every gated tool with a plan_step argument and rejects the unknown-field error, so the next strict tool cannot regress the same way.

## Acceptance Criteria

- A web call carrying plan_step decodes instead of failing with unknown field plan_step.
- A regression test proves every plan-gated tool accepts plan_step in its arguments.
- CHANGELOG records the fix under Unreleased.

## Verification Plan

1. go test ./internal/tools/ ./internal/tools/webtool/
2. scoped golangci-lint run over internal/tools/...
