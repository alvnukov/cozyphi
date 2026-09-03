---
id: sandbox-mcp-stdio-environment
title: Sandbox MCP stdio processes and stop ambient secret inheritance
status: todo
priority: high
model_level: high
task_type: feature
parent_id: harness-security-hardening
tags:
    - security
    - mcp
    - secrets
    - sandbox
acceptance_criteria:
    - MCP child processes receive a documented minimal environment by default.
    - Secrets are injected only by explicit named configuration and never printed.
    - MCP config and logs are created/tightened to 0600.
    - Sandbox capability degrades explicitly rather than silently.
verification_plan:
    - Child-process fixture records env; tests assert ambient secrets absent, explicit vars present, and file modes are 0600.
created_at: "2026-08-24T13:20:17.834485Z"
updated_at: "2026-08-24T13:20:17.834485Z"
---

## Body

internal/mcp/stdio.go launches configured binaries with os.Environ(), exposing every ambient API token/credential to each MCP server, and writes stderr logs with mode 0644. Replace ambient inheritance with a minimal environment and explicit secret handles/allowlist; tighten existing logs and config containing headers/env to 0600; evaluate platform sandboxing for filesystem/network scope.

## Acceptance Criteria

- MCP child processes receive a documented minimal environment by default.
- Secrets are injected only by explicit named configuration and never printed.
- MCP config and logs are created/tightened to 0600.
- Sandbox capability degrades explicitly rather than silently.

## Verification Plan

1. Child-process fixture records env; tests assert ambient secrets absent, explicit vars present, and file modes are 0600.
