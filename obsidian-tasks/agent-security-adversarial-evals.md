---
id: agent-security-adversarial-evals
title: Add repeatable adversarial security evals for the harness
status: todo
priority: high
model_level: high
task_type: test
parent_id: harness-security-hardening
tags:
    - security
    - evals
    - prompt-injection
    - regression
acceptance_criteria:
    - CI runs deterministic control-plane security tests on every permission/tool change.
    - Model-dependent evals report per-attack and repeated-attempt success without gating deterministic unit tests on network models.
    - Previously fixed exploit chains remain versioned and cannot be weakened silently.
verification_plan:
    - Run deterministic corpus locally/CI; produce bounded report for model-backed repeated trials.
created_at: "2026-08-24T13:20:17.843656Z"
updated_at: "2026-08-24T13:20:17.843656Z"
---

## Body

Create a deterministic attack corpus and harness-level evals covering repository instruction injection, poisoned tool output/descriptions, config escalation, secret exfiltration, approval bypass, recursive retries, multi-agent propagation, and supply-chain hooks/MCP. Measure attack success over repeated attempts as recommended by current agent-hijacking evaluation guidance.

## Acceptance Criteria

- CI runs deterministic control-plane security tests on every permission/tool change.
- Model-dependent evals report per-attack and repeated-attempt success without gating deterministic unit tests on network models.
- Previously fixed exploit chains remain versioned and cannot be weakened silently.

## Verification Plan

1. Run deterministic corpus locally/CI; produce bounded report for model-backed repeated trials.
