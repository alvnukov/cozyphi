---
id: enforce-agent-dataflow-policy
title: 04 — Gate confidential actions with recipient-bound user grants
status: blocked
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
    - Host provenance drives deterministic action policy across builtins, MCP, hooks, watches and children; model text cannot forge consent.
    - Grants default to one use and bind data/version, sink, action and expiry or use count; task grants require separate bounded approval.
    - Checks see final rewritten arguments and invalidate stale approvals; hard denies and the ordinary permission gate cannot be bypassed.
    - Prehook launch, nested MCP arguments and background lifetimes are covered where visible; V1 does not claim arbitrary process-egress DLP.
    - UI/headless decisions explain source, sink and scope without secrets; missing approval handler stops and concurrent/revoked grants cannot replay.
verification_plan:
    - Exercise S05 and S06 through actual executor adapters with controlled publication sinks and synthetic data.
    - Cover public issue to private file to public PR, webpage to shell, MCP result to credential read, argument rewrite and concurrent grant use.
    - Run only changed-package tests and warranted race checks plus one scoped lint; broad gates remain in CI.
created_at: "2026-08-24T13:20:17.836964Z"
updated_at: "2026-09-09T11:16:39Z"
---

## Body

**What to build:** A user completes an intended confidential action under a narrowly scoped recipient grant, while a poisoned input cannot publish to a different sink or reuse altered arguments. This refines the existing toxic-flow task rather than creating a duplicate.

**Blocked by:** security-durable-provenance.

**Contract:** [Specification](../specs/harness-security.md), D2–D4; slice 04 of the [approved map](../specs/harness-security-tickets.md).

**Scope:** Use host provenance from slice 02; preserve PreHooks, final-argument Plan/Security/Permission/Ask, Run and PostHooks. Control a PreHook launch as its own action before running it. MCP discovery may launch code or contact a server. Watch lifetimes and read-only child model recipients are not exemptions. Raw process isolation is separate stage-two work; visible-action mediation must disclose that limit.

**Blocked (2026-09-09):** Waiting for complete ingress/lifecycle provenance. The previous broad provenance requirement is owned by slice 02 and remains a prerequisite rather than duplicated implementation.

## Acceptance Criteria

- Host provenance drives deterministic action policy across builtins, MCP, hooks, watches and children; model text cannot forge consent.
- Grants default to one use and bind data/version, sink, action and expiry or use count; task grants require separate bounded approval.
- Checks see final rewritten arguments and invalidate stale approvals; hard denies and the ordinary permission gate cannot be bypassed.
- Prehook launch, nested MCP arguments and background lifetimes are covered where visible; V1 does not claim arbitrary process-egress DLP.
- UI/headless decisions explain source, sink and scope without secrets; missing approval handler stops and concurrent/revoked grants cannot replay.

## Verification Plan

1. Exercise S05 and S06 through actual executor adapters with controlled publication sinks and synthetic data.
2. Cover public issue to private file to public PR, webpage to shell, MCP result to credential read, argument rewrite and concurrent grant use.
3. Run only changed-package tests and warranted race checks plus one scoped lint; broad gates remain in CI.
