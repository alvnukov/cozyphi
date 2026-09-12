---
id: security-process-isolation-design
title: 13 — Design process isolation as the second security stage
status: todo
priority: high
model_level: high
task_type: design
parent_id: harness-security-hardening
tags: [security, sandbox, design]
acceptance_criteria:
  - The design maps shell, hooks, MCP and watches to process-tree, environment, filesystem and network boundaries.
  - Platform capabilities and unsupported behavior are evidenced and surfaced explicitly rather than silently downgraded.
  - Lifecycle handling includes cancellation, descendants, background watches, prehook launch and secret-safe diagnostics.
  - Existing subprocess and MCP sandbox tasks retain their ownership and are linked instead of reimplemented by this design task.
  - A reviewed next-stage contract and demonstrable acceptance scenarios precede any process-egress prevention claim.
verification_plan:
  - Trace current launch paths and compare platform primitives using authoritative documentation.
  - Review S17 and synthetic network/filesystem/environment/orphan-process scenarios at the public process interface, including UI/headless explanations for unsupported isolation.
  - Validate the design and dependency links only; no process sandbox implementation, deployment or broad Go checks in this task.
created_at: "2026-09-09T11:16:39Z"
updated_at: "2026-09-09T11:16:39Z"
---

## Body

**What to build:** A user-reviewable second-stage architecture that says which subprocess effects can be contained, on which platforms, and what happens when isolation is unavailable. The deliverable is a design and acceptance walkthrough, not a sandbox implementation.

**Blocked by:** None — can start immediately. V1 delivery is not needed to research the process boundary.

**Contract:** [Specification](../specs/harness-security.md), D4 and D9; slice 13 of the [approved map](../specs/harness-security-tickets.md).

**Related implementation owners:** [Managed subprocess seam](refactor-external-binary-runner.md) and [MCP environment/sandbox](sandbox-mcp-stdio-environment.md). Assess their current state and identify any remaining future slices without modifying or duplicating their scope in this task. Neither related task is a blocker for producing the design.

**Scope:** Process isolation is the second major stage. Account for environment credentials, network/FS policy, protocol servers, recursive process trees and background lifetimes. V1 visible-action checks are not process-egress DLP. OS compromise and trusted-server behavior remain explicit limits.

## Acceptance Criteria

- The design maps shell, hooks, MCP and watches to process-tree, environment, filesystem and network boundaries.
- Platform capabilities and unsupported behavior are evidenced and surfaced explicitly rather than silently downgraded.
- Lifecycle handling includes cancellation, descendants, background watches, prehook launch and secret-safe diagnostics.
- Existing subprocess and MCP sandbox tasks retain their ownership and are linked instead of reimplemented by this design task.
- A reviewed next-stage contract and demonstrable acceptance scenarios precede any process-egress prevention claim.

## Verification Plan

1. Trace current launch paths and compare platform primitives using authoritative documentation.
2. Review S17 and synthetic network/filesystem/environment/orphan-process scenarios at the public process interface, including UI/headless explanations for unsupported isolation.
3. Validate the design and dependency links only; no process sandbox implementation, deployment or broad Go checks in this task.
