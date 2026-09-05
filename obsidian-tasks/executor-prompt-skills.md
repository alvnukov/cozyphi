---
id: executor-prompt-skills
title: Teach parent and executor the approved review protocol
status: blocked
priority: medium
model_level: medium
task_type: feature
parent_id: plan-driven-interactive-executors
tags:
    - executors
    - prompts
    - skills
acceptance_criteria:
    - Model-facing executor guidance proposes a local plan and waits for parent approval before execution.
    - Parent guidance separates approach approval from result acceptance and uses retained-session rework.
    - Parent messages do not impersonate human permission approval.
    - Skills cannot restore disabled capabilities or bypass runtime guards.
    - Permanent identity rules and selected role skills are wired into actual prompt assembly; capability and reasoning effort remain distinct.
verification_plan:
    - Test prompt assembly for each role/mode and selected/disabled skills.
    - Run a scripted propose, return, approve and rework walkthrough.
    - Check adversarial quoted approval and missing-context examples against the required explicit instructions.
created_at: "2026-09-05T23:22:04.431706Z"
updated_at: "2026-09-05T23:22:04.431706Z"
---

## Body

**Approved slice:** 11 of [breakdown](../specs/plan-driven-executors-tickets.md). Read [contract](../specs/plan-driven-executors.md), D10.

**Blocked by:** executor-human-authority, executor-independent-review.

**Capability rationale:** medium — roles require consistent operational guidance, but runtime semantics and public operations are settled.

**Inputs:** finalized schemas/denial messages, accepted scenarios from slices 02–06 and project agent-writing guidelines.

**Scope/output:** implement permanent identity/authority rules plus selectable parent, executor and independent-review skill workflows in actual prompt assembly. Include capability-specific briefs, escalation examples and context-update usage.

**Escalation:** unclear semantics return to foundation owners. Security guarantees belong in runtime enforcement, not prompt-only patches; prose never adds tool authority.

## Acceptance Criteria

- Model-facing executor guidance proposes a local plan and waits for parent approval before execution.
- Parent guidance separates approach approval from result acceptance and uses retained-session rework.
- Parent messages do not impersonate human permission approval.
- Skills cannot restore disabled capabilities or bypass runtime guards.
- Permanent identity rules and selected role skills are wired into actual prompt assembly; capability and reasoning effort remain distinct.

## Verification Plan

1. Test prompt assembly for each role/mode and selected/disabled skills.
2. Run a scripted propose, return, approve and rework walkthrough.
3. Check adversarial quoted approval and missing-context examples against the required explicit instructions.
