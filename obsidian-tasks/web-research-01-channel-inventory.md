---
id: web-research-01-channel-inventory
title: 01 — Inventory protected-web bypass channels and platform evidence
status: todo
priority: high
model_level: low
task_type: docs
parent_id: web-tools
tags:
    - protected-web-research
    - ready-for-agent
acceptance_criteria:
    - Inventory covers ordinary file tools, shell/Python, MCP, hooks, watches, delegated work, model requests, cache/transcripts and all supported product platforms.
    - Each enabled route has implementation evidence and a public boundary test, or is explicitly unproved and unavailable for protected web.
    - Unresolved platform/coordination ownership and missing implementation task IDs are listed; design completion is never isolation proof.
verification_plan:
    - Compare the inventory against every D7 channel and the actual supported-platform configuration.
    - Read linked task states and distinguish design, implementation and verification.
    - Validate local links and git diff --check only; do not run Go gates or live attacks for this documentation task.
created_at: "2026-09-19T19:10:37.596089Z"
updated_at: "2026-09-19T19:10:37.596089Z"
---

## Body

**What to build:** A bounded evidence inventory that tells the next executor which existing channels can bypass web quarantine and what must be disabled or implemented first.

**Blocked by:** None — can start immediately.

**Contract:** [Protected web spec](../specs/protected-web-research.md), D7/D15/D16, Q13/Q26/Q39, T13/T30. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Enumerate actual product platforms and launch/read/egress entry points using repository configuration and semantic references. For each, record owner, applicable shared security task, current enforcement, reproducible test and residual gap. Assess existing host coordination rather than invent a second global service. Distinguish landed code from unmerged worktrees. Link [process-isolation design](security-process-isolation-design.md), managed runner and MCP environment owners. Produce the matrix and a list of missing implementation blockers; do not implement them here.

**Do not change:** Runtime, security settings, parent epic, other tasks or installed configuration.

**Proof required:** Commit-bound inventory with source references, platform list and ready/disabled/unproved classifications; an independent reviewer can reproduce every claimed barrier. Unknown is an acceptable result, fabricated readiness is not.

**Stop condition:** If a platform or owner cannot be established, record the missing evidence and block downstream enablement.

## Acceptance Criteria

- Inventory covers ordinary file tools, shell/Python, MCP, hooks, watches, delegated work, model requests, cache/transcripts and all supported product platforms.
- Each enabled route has implementation evidence and a public boundary test, or is explicitly unproved and unavailable for protected web.
- Unresolved platform/coordination ownership and missing implementation task IDs are listed; design completion is never isolation proof.

## Verification Plan

1. Compare the inventory against every D7 channel and the actual supported-platform configuration.
2. Read linked task states and distinguish design, implementation and verification.
3. Validate local links and git diff --check only; do not run Go gates or live attacks for this documentation task.
