---
id: secure-project-mcp-config-trust
title: Require explicit trust for executable project MCP configuration
status: todo
priority: critical
model_level: high
task_type: feature
parent_id: harness-security-hardening
tags:
    - security
    - mcp
    - rce
    - supply-chain
acceptance_criteria:
    - Project MCP entries are inert until exact configuration is explicitly approved.
    - Project entries cannot shadow same-named user servers.
    - Approval displays and binds source plus exact argv or URL/headers-with-secrets-redacted.
    - Changing approved executable configuration invalidates approval.
verification_plan:
    - Tests cover drive-by project stdio config, name shadowing, config hash change, headless fail-closed behavior.
created_at: "2026-08-24T13:20:17.83167Z"
updated_at: "2026-08-24T13:20:17.83167Z"
---

## Body

internal/mcp/config.go merges <repo>/.cozyphi/mcp.json after ~/.cozyphi/mcp.json, so an untrusted repository can replace a trusted same-named server. A later mcp_list/inspect/call lazily executes the repository-provided stdio command; the approval UI does not bind consent to the exact argv/source. Introduce source-aware config, forbid project shadowing, and require explicit per-project approval bound to a config hash before any project stdio spawn or HTTP connection. Re-approval is required after changes (rug-pull defense).

## Acceptance Criteria

- Project MCP entries are inert until exact configuration is explicitly approved.
- Project entries cannot shadow same-named user servers.
- Approval displays and binds source plus exact argv or URL/headers-with-secrets-redacted.
- Changing approved executable configuration invalidates approval.

## Verification Plan

1. Tests cover drive-by project stdio config, name shadowing, config hash change, headless fail-closed behavior.
