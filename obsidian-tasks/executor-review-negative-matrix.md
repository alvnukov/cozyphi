---
id: executor-review-negative-matrix
title: Extend the fixed executor review rejection matrix
status: blocked
priority: medium
model_level: low
task_type: test
parent_id: plan-driven-interactive-executors
tags:
    - executors
    - tests
acceptance_criteria:
    - The prescribed wrong-parent, stale-plan, changed-assignment, duplicate-approval, duplicate-result, unapproved-result and completed-but-unaccepted cases are covered.
    - Every case asserts the expected visible rejection/idempotent response and unchanged parent step.
    - Tests use public operations and deterministic supplied fixtures.
    - This slice changes test code only; foundational correctness was already tested by its prerequisites.
verification_plan:
    - Run the narrow added matrix, then existing related suites.
    - Verify a deliberate local expectation mutation fails; restore it before committing.
    - Inspect the diff to confirm test-only scope and assertion of unchanged parent state.
created_at: "2026-09-05T23:22:18.347205Z"
updated_at: "2026-09-05T23:22:18.347205Z"
---

## Body

**Approved slice:** 12 of [breakdown](../specs/plan-driven-executors-tickets.md). Read [contract](../specs/plan-driven-executors.md) approval and acceptance rules.

**Blocked by:** executor-result-acceptance.

**Capability rationale:** low — expand a prescribed test matrix using existing public helpers and fixed expected results, not new infrastructure or protocol design.

**Inputs:** slices 02–03 public test helpers and approved expected cases: wrong parent, stale local-plan revision, changed assignment, duplicate approval, duplicate result, result without approval, completed job without parent acceptance.

**Scope/output:** table-driven tests for those exact cases; no production changes.

**Escalation:** a production defect or missing fixture returns with reproduction to the foundation owner; do not repair authority code or relax expectations in this task.

## Acceptance Criteria

- The prescribed wrong-parent, stale-plan, changed-assignment, duplicate-approval, duplicate-result, unapproved-result and completed-but-unaccepted cases are covered.
- Every case asserts the expected visible rejection/idempotent response and unchanged parent step.
- Tests use public operations and deterministic supplied fixtures.
- This slice changes test code only; foundational correctness was already tested by its prerequisites.

## Verification Plan

1. Run the narrow added matrix, then existing related suites.
2. Verify a deliberate local expectation mutation fails; restore it before committing.
3. Inspect the diff to confirm test-only scope and assertion of unchanged parent state.
