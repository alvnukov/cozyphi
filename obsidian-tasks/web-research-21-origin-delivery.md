---
id: web-research-21-origin-delivery
title: 21 — Deliver checked completion only to its current originating request
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Completion is bound to original session/request/workspace and delivered at most once.
    - An active turn queues completion rather than accepting a mid-turn insertion; tab switching does not redirect it.
    - Cancelled, superseded or stale requests produce only safe user status, never a revived model assignment.
verification_plan:
    - Use two sessions and controlled turn/completion barriers for active-turn, switch, supersede and duplicate cases.
    - Observe actual parent messages and user notifications, asserting exactly-one or zero delivery as appropriate.
    - Run scoped lifecycle/controller race tests without sleep-based ordering.
created_at: "2026-09-19T19:15:57.542346Z"
updated_at: "2026-09-19T19:15:57.542346Z"
---

## Body

**What to build:** Origin-bound completion delivery using the existing session/job bus.

**Blocked by:** [08](web-research-08-release-races.md), [20](web-research-20-safe-progress.md).

**Contract:** [Spec](../specs/protected-web-research.md), D12, Q17/Q21/Q24, T26. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Carry immutable originating assignment identity through submission, completion and consumption. Revalidate current relevance and release eligibility when delivering after a queued turn. Handle duplicate completion, tab changes and cancellation through existing lifecycle transitions. Show stale outcomes to the user without inserting them as new instructions.

**Do not change:** No new daemon, active-tab routing shortcut, polling requirement for the main model or bypass of the release barrier.

**Proof required:** Two-session test with active-turn barriers and a switched tab; inspect each session's actual messages. Duplicate events yield one result, stale/cancelled events zero model deliveries. Record safe user notifications separately.

**Stop condition:** Missing durable assignment identity is a shared lifecycle prerequisite, not permission to route by whichever session is currently visible.

## Acceptance Criteria

- Completion is bound to original session/request/workspace and delivered at most once.
- An active turn queues completion rather than accepting a mid-turn insertion; tab switching does not redirect it.
- Cancelled, superseded or stale requests produce only safe user status, never a revived model assignment.

## Verification Plan

1. Use two sessions and controlled turn/completion barriers for active-turn, switch, supersede and duplicate cases.
2. Observe actual parent messages and user notifications, asserting exactly-one or zero delivery as appropriate.
3. Run scoped lifecycle/controller race tests without sleep-based ordering.
