---
id: harness-security-hardening
title: Harden the agent harness against injection and sandbox escapes
status: todo
priority: high
model_level: high
task_type: epic
parent_id: cozyphi-enterprise-code-review
tags:
    - security
    - hardening
acceptance_criteria:
    - A repository-specific threat model maps current attack vectors to concrete trust boundaries and existing controls.
    - At least one confirmed high-impact vulnerability cluster is fixed at a central seam with regression tests.
    - Remaining confirmed findings are recorded as prioritized child tasks with acceptance criteria.
    - No permission bypass, silent failure, secret logging, unsafe path resolution, or unbounded parser/process behavior is introduced.
verification_plan:
    - Run the narrow security regression tests for each touched seam.
    - Run adjacent package tests and race-sensitive tests where concurrency or process handling changes.
    - Run make fmt-check, make lint, and make test before atomic task close and owned-files commit.
created_at: "2026-08-24T13:06:13.327081Z"
updated_at: "2026-08-25T22:32:15.864596Z"
---

## Body

Umbrella epic for harness security hardening. Recreated because several tasks (permission-symlink-workspace-escape, remove-executable-default-bash-allowlist, secure-project-mcp-config-trust, agent-security-adversarial-evals) referenced it as parent while the note itself was missing, which blocked every registry write with "parent harness-security-hardening does not exist". Children carry the actual scope.

## Acceptance Criteria

- A repository-specific threat model maps current attack vectors to concrete trust boundaries and existing controls.
- At least one confirmed high-impact vulnerability cluster is fixed at a central seam with regression tests.
- Remaining confirmed findings are recorded as prioritized child tasks with acceptance criteria.
- No permission bypass, silent failure, secret logging, unsafe path resolution, or unbounded parser/process behavior is introduced.

## Verification Plan

1. Run the narrow security regression tests for each touched seam.
2. Run adjacent package tests and race-sensitive tests where concurrency or process handling changes.
3. Run make fmt-check, make lint, and make test before atomic task close and owned-files commit.
