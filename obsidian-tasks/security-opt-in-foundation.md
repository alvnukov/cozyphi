---
id: security-opt-in-foundation
title: 01 — Enable security observation for a first protected read
status: todo
priority: high
model_level: high
task_type: feature
parent_id: harness-security-hardening
tags: [security, prompt-injection]
acceptance_criteria:
  - Missing configuration is off with zero new checks, asks, audit or shadow work; existing controls remain.
  - Only explicit human configuration enables observe; ordinary permission bypass and injected text cannot change security state.
  - A confidential file read reaches a controlled model only under a recipient-bound user decision and deterministic policy.
  - UI and headless responses disclose mode, partial coverage and refusal reasons without sensitive content.
  - Turning off cancels pending security work without automatically replaying suspended actions.
verification_plan:
  - Exercise S01, S02 and S13 through the public engine and fake provider with call and output captures.
  - Test off, first enablement, forged settings, denied recipient, cancellation and absent approval handler.
  - Run only changed-package tests and one scoped lint before commit; leave full gates to CI.
created_at: "2026-09-09T11:16:39Z"
updated_at: "2026-09-09T11:16:39Z"
---

## Body

**What to build:** A user can enable observe and complete one confidential read-to-model flow with an explicit recipient decision, or keep legacy behavior with security off. Establish the small policy interface using this real adapter path, not a framework without behavior.

**Blocked by:** None — can start immediately.

**Contract:** [Specification](../specs/harness-security.md), D1–D3; slice 01 of the [approved map](../specs/harness-security-tickets.md).

**Scope:** User-owned settings, mode indicator, host material identity, a first deterministic recipient decision, cancellation and observable tests. Partial coverage is explicit; full enforcement is unavailable. Model checking is not needed to demonstrate this slice. Later slices expand the same interface, rather than duplicating permission logic.

## Acceptance Criteria

- Missing configuration is off with zero new checks, asks, audit or shadow work; existing controls remain.
- Only explicit human configuration enables observe; ordinary permission bypass and injected text cannot change security state.
- A confidential file read reaches a controlled model only under a recipient-bound user decision and deterministic policy.
- UI and headless responses disclose mode, partial coverage and refusal reasons without sensitive content.
- Turning off cancels pending security work without automatically replaying suspended actions.

## Verification Plan

1. Exercise S01, S02 and S13 through the public engine and fake provider with call and output captures.
2. Test off, first enablement, forged settings, denied recipient, cancellation and absent approval handler.
3. Run only changed-package tests and one scoped lint before commit; leave full gates to CI.
