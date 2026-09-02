---
id: remove-executable-default-bash-allowlist
title: Remove code-executing commands from default Bash auto-allow
status: todo
priority: critical
model_level: high
task_type: bug
parent_id: harness-security-hardening
tags:
    - security
    - permissions
    - rce
    - prompt-injection
acceptance_criteria:
    - Commands that execute repository-controlled code never receive default Allow.
    - Approval is parameter-bound and shown after complete command parsing.
    - Writes to execution-control files such as .git/config and hook directories require explicit approval.
    - Read-only commands retained in the allowlist have adversarial parsing tests.
verification_plan:
    - Table-driven tests cover go test/build/generate, compound syntax, config-write then command chains, and readonly modes.
created_at: "2026-08-24T13:20:17.833178Z"
updated_at: "2026-08-24T13:20:17.833178Z"
---

## Body

internal/permission/defaults.go auto-allows go test and go build. Repository-controlled tests/build inputs can execute code, so indirect prompt injection can combine an allowed workspace write with an automatically allowed test/build and reach host RCE without a fresh approval. Classify commands by effects; executable build/test/package lifecycle commands must Ask by default. Protect harness and VCS control files that can alter later execution.

## Acceptance Criteria

- Commands that execute repository-controlled code never receive default Allow.
- Approval is parameter-bound and shown after complete command parsing.
- Writes to execution-control files such as .git/config and hook directories require explicit approval.
- Read-only commands retained in the allowlist have adversarial parsing tests.

## Verification Plan

1. Table-driven tests cover go test/build/generate, compound syntax, config-write then command chains, and readonly modes.
