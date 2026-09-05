---
id: executor-budget-regressions
title: Extend deterministic executor context-budget regressions
status: blocked
priority: medium
model_level: low
task_type: test
parent_id: plan-driven-interactive-executors
tags:
    - executors
    - tests
    - context
acceptance_criteria:
    - Exact fit, one-token overflow, output reserve, rejected compaction and unchanged parent projection cases match prescribed expectations.
    - No case silently launches prompt-only; fitting input is preserved.
    - Parent snapshot identity/content stays unchanged after retry.
    - Only tests and supplied expected messages change; no token estimation, compaction or provider behavior changes.
verification_plan:
    - Run the targeted matrix and existing context regression suite.
    - Verify a deliberate local expectation mutation fails and restore it before committing.
    - Inspect test-only scope and exact too-large message behavior.
created_at: "2026-09-05T23:22:31.009803Z"
updated_at: "2026-09-05T23:22:31.009803Z"
---

## Body

**Approved slice:** 13 of [breakdown](../specs/plan-driven-executors-tickets.md). Read [contract](../specs/plan-driven-executors.md) context-budget rules.

**Blocked by:** executor-context-budget.

**Capability rationale:** low — deterministic fixture additions against an accepted budget interface and fixed accounting.

**Inputs:** slice 07 public fixtures and expected cases for exact fit, one-token overflow, output reserve, rejected compaction and unchanged parent projection.

**Scope/output:** test-only matrix covering those cases and the specified too-large message. Keep production compaction, token estimation and provider behavior unchanged.

**Escalation:** tokenizer nondeterminism or a production defect returns to the budget owner; never weaken preservation expectations to make tests pass.

## Acceptance Criteria

- Exact fit, one-token overflow, output reserve, rejected compaction and unchanged parent projection cases match prescribed expectations.
- No case silently launches prompt-only; fitting input is preserved.
- Parent snapshot identity/content stays unchanged after retry.
- Only tests and supplied expected messages change; no token estimation, compaction or provider behavior changes.

## Verification Plan

1. Run the targeted matrix and existing context regression suite.
2. Verify a deliberate local expectation mutation fails and restore it before committing.
3. Inspect test-only scope and exact too-large message behavior.
