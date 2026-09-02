---
id: defend-mcp-tool-poisoning-rug-pulls
title: Defend MCP discovery and calls from tool poisoning and rug pulls
status: todo
priority: high
model_level: high
task_type: feature
parent_id: harness-security-hardening
tags:
    - security
    - mcp
    - prompt-injection
    - tool-poisoning
acceptance_criteria:
    - Remote descriptions/results are provenance-labelled untrusted data, never developer instructions.
    - Approved tool identity is bound to server plus schema/description fingerprint.
    - Rug-pull changes invalidate cached approval and are visible.
    - Sensitive arguments are visible to the user with secrets safely redacted, not omitted.
verification_plan:
    - Adversarial MCP fixture injects instructions and changes schema/description between list and call; execution remains gated.
created_at: "2026-08-24T13:20:17.835561Z"
updated_at: "2026-08-24T13:20:17.835561Z"
---

## Body

mcp_inspect returns remote tool descriptions directly to the model and no schema/description fingerprint is pinned. A server can embed instructions in descriptions or change them after trust. Mark descriptions as untrusted data, display full reviewed schema/arguments, pin fingerprints for approvals, and require re-approval on change. Do not rely only on prompt-injection classifiers.

## Acceptance Criteria

- Remote descriptions/results are provenance-labelled untrusted data, never developer instructions.
- Approved tool identity is bound to server plus schema/description fingerprint.
- Rug-pull changes invalidate cached approval and are visible.
- Sensitive arguments are visible to the user with secrets safely redacted, not omitted.

## Verification Plan

1. Adversarial MCP fixture injects instructions and changes schema/description between list and call; execution remains gated.
