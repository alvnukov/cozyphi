---
id: fail-closed-permission-assembly
title: Make incomplete permission assembly fail closed
status: todo
priority: high
model_level: high
task_type: bug
parent_id: harness-security-hardening
tags:
    - security
    - permissions
    - fail-closed
acceptance_criteria:
    - Nil or incomplete gate assembly returns Deny, never Allow.
    - Only an explicit enabled bypass can return unconditional Allow.
    - Controller/run reconfiguration tests prove there is no transient fail-open window.
verification_plan:
    - Unit tests cover nil receiver/inner/enabled combinations; integration test races/reconfigures the engine while requests arrive.
created_at: "2026-08-24T13:20:17.83933Z"
updated_at: "2026-08-24T13:20:17.83933Z"
---

## Body

internal/permission/bypass.go returns Allow when BypassGate or its Inner gate is nil. Any incomplete/reconfigured assembly silently removes the permission boundary. Return Deny with an actionable reason unless bypass is explicitly enabled by user state, and assert every production constructor installs a complete gate.

## Acceptance Criteria

- Nil or incomplete gate assembly returns Deny, never Allow.
- Only an explicit enabled bypass can return unconditional Allow.
- Controller/run reconfiguration tests prove there is no transient fail-open window.

## Verification Plan

1. Unit tests cover nil receiver/inner/enabled combinations; integration test races/reconfigures the engine while requests arrive.
