---
id: web-research-01-channel-inventory
title: 01 — Inventory protected-web bypass channels and platform evidence
status: in_progress
priority: high
model_level: low
task_type: docs
parent_id: web-tools
tags:
    - protected-web-research
    - ready-for-agent
branch: docs/web-research-01-channel-inventory
worktree_path: .worktrees/web-research-01-channel-inventory
acceptance_criteria:
    - Inventory covers ordinary file tools, shell/Python, MCP, hooks, watches, delegated work, model requests, cache/transcripts and all supported product platforms.
    - Each enabled route has implementation evidence and a public boundary test, or is explicitly unproved and unavailable for protected web.
    - Unresolved platform/coordination ownership and missing implementation task IDs are listed; design completion is never isolation proof.
verification_plan:
    - Compare the inventory against every D7 channel and the actual supported-platform configuration.
    - Read linked task states and distinguish design, implementation and verification.
    - Validate local links and git diff --check only; do not run Go gates or live attacks for this documentation task.
created_at: "2026-09-19T19:10:37.596089Z"
updated_at: "2026-09-19T21:34:23.256048Z"
---

## Body

**What to build:** A bounded evidence inventory that tells the next executor which existing channels can bypass web quarantine and what must be disabled or implemented first.

**Blocked by:** None — can start immediately.

**Contract:** [Protected web spec](../specs/protected-web-research.md), D7/D15/D16, Q13/Q26/Q39, T13/T30. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Enumerate actual product platforms and launch/read/egress entry points using repository configuration and semantic references. For each, record owner, applicable shared security task, current enforcement, reproducible test and residual gap. Assess existing host coordination rather than invent a second global service. Distinguish landed code from unmerged worktrees. Link [process-isolation design](security-process-isolation-design.md), managed runner and MCP environment owners. Produce the matrix and a list of missing implementation blockers; do not implement them here.

**Do not change:** Runtime, security settings, parent epic, other tasks or installed configuration.

**Proof required:** Commit-bound inventory with source references, platform list and ready/disabled/unproved classifications; an independent reviewer can reproduce every claimed barrier. Unknown is an acceptable result, fabricated readiness is not.

**Stop condition:** If a platform or owner cannot be established, record the missing evidence and block downstream enablement.

**Started (2026-09-19).** Первая задача эпика после публикации тикетов: инвентаризация каналов обхода web-карантина. Только docs/расследование, рантайм не трогаем.

**Note (2026-09-20).** Delivered specs/protected-web-channel-inventory.md at branch docs/web-research-01-channel-inventory (base 71b2277d). Evidence inventory: 10 channels C1–C10 (file tools, shell/Python, MCP, hooks, watches, sub-agents, model/web ingress, caches/persistence, diagnostics), host-coordination assessment (per-process gate chain TaintGate→BypassGate→StaticGate; no cross-process coordination; H host-coordination owner unidentified), platform matrix (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 — all unproved). Classifications: 4 ready (narrowly scoped), 1 disabled (quarantine:off), rest unproved with named missing evidence. Executed by delegated worker job_20260919T205910_a18286b2c2c301d9; independently verified by review job_20260919T211610_7e7685aa3c3ffa3c (~35 file:line spot checks, all 3 acceptance criteria PASS; 5 nit discrepancies found and fixed: off-by-one citations in internal/hooks/command.go/.goreleaser.yaml/ci.yml, overbroad config.go parenthetical, missing D7 incident-views note). Docs gates: all relative links resolve, git diff --check clean; no Go gates (docs-only task). Local commit only — PR/merge pending explicit user command.

## Acceptance Criteria

- Inventory covers ordinary file tools, shell/Python, MCP, hooks, watches, delegated work, model requests, cache/transcripts and all supported product platforms.
- Each enabled route has implementation evidence and a public boundary test, or is explicitly unproved and unavailable for protected web.
- Unresolved platform/coordination ownership and missing implementation task IDs are listed; design completion is never isolation proof.

## Verification Plan

1. Compare the inventory against every D7 channel and the actual supported-platform configuration.
2. Read linked task states and distinguish design, implementation and verification.
3. Validate local links and git diff --check only; do not run Go gates or live attacks for this documentation task.
