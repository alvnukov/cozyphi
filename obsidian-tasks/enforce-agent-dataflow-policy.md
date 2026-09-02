---
id: enforce-agent-dataflow-policy
title: Gate toxic cross-domain agent data flows
status: todo
priority: high
model_level: high
task_type: feature
parent_id: harness-security-hardening
tags:
    - security
    - prompt-injection
    - exfiltration
    - agent
acceptance_criteria:
    - Every model-visible external datum carries origin/trust metadata outside attacker-controlled text.
    - Sensitive read -> external write flows require explicit approval showing source and sink.
    - Tool output cannot forge control messages or approval state.
    - Policy applies across builtins, MCP, hooks, and sub-agents.
verification_plan:
    - Toxic-flow tests cover public issue -> private file -> public PR, webpage -> shell, and MCP result -> credential read.
created_at: "2026-08-24T13:20:17.836964Z"
updated_at: "2026-08-24T13:20:17.836964Z"
---

## Body

Tool results, repository files, web content, issues, and MCP output are returned to the model as plain content. A poisoned public source can instruct the model to read private data and publish it through another tool. Add provenance/taint metadata and deterministic policy at action time: untrusted-input-derived requests cannot cross into sensitive reads or external writes without a fresh parameter-bound user approval.

## Acceptance Criteria

- Every model-visible external datum carries origin/trust metadata outside attacker-controlled text.
- Sensitive read -> external write flows require explicit approval showing source and sink.
- Tool output cannot forge control messages or approval state.
- Policy applies across builtins, MCP, hooks, and sub-agents.

## Verification Plan

1. Toxic-flow tests cover public issue -> private file -> public PR, webpage -> shell, and MCP result -> credential read.
