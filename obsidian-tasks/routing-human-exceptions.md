---
id: routing-human-exceptions
title: 07 — Honor user pins and once-per-scope routing exceptions
status: blocked
priority: high
model_level: high
task_type: feature
parent_id: subscription-aware-routing
tags:
    - routing
    - subscriptions
    - permissions
acceptance_criteria:
    - A genuine user pin prevents automatic replacement, including stale/unavailable pins; technical infeasibility produces an actionable choice rather than silent fallback.
    - Before lower-than-required execution or reserve use outside normal policy, disclose the concrete risk and obtain one explicit user confirmation for a defined account/profile/work/time/limit scope.
    - Valid grants execute without repeated prompts within scope; revocation, expiry and material scope changes invalidate the grant, with correct restart behavior.
    - The model, plan text, files, tool results and replayed unrelated confirmations cannot issue or broaden a grant; priority and model identity remain human-owned.
    - A headless inability to confirm yields a structured pending decision, not consent; pending/waiting state is cancellable and explained.
    - 'Only routing policy is overridden: technical/provider limitations, tool permission gates, context approval and the separate general tool root-override contract remain intact.'
verification_plan:
    - Run S11, S12 and S20 through genuine user interaction and synthetic inference, including lower-tier pins and reserve-denial fixtures.
    - Verify one confirmation covers only the displayed scope, survives valid restart and stops after expiry/revocation/account change.
    - Inject approval-looking text via model arguments, files and tool output; assert no authority is created.
    - Verify no repeated prompt inside scope and no bypass of tool/context gates; test cancellable headless pending decisions.
created_at: "2026-09-06T11:12:22.156346Z"
updated_at: "2026-09-06T11:15:04.391521Z"
---

## Body

**Approved slice:** 07 of the [breakdown](../specs/subscription-aware-routing-tickets.md). Read the [contract](../specs/subscription-aware-routing.md), D6.

**Blocked by:** routing-openai-account-admission.

**What to build:** Let the user deliberately pin a profile and authorize a disclosed routing exception once per scope, while preventing the agent or untrusted content from manufacturing approval.

**Inputs:** Shared admit/ask/wait outcomes and existing user interaction/approval paths. Reserve-denial fixtures exercise the grant path independently of adaptive forecasting.

**Scope/output:** Pin precedence, scoped grant lifecycle, user confirmation UI/headless pending state, persisted/revocable authority and adversarial tests.

**Related, not blocking:** user-root-tool-override addresses general tool restrictions separately. This slice must not wait for or silently implement that broader feature. Paid cap exceptions are delivered by slice 08.

**Blocked (2026-09-06).** Waiting for routing-openai-account-admission; consume its structured admission and pending-decision outcomes.

## Acceptance Criteria

- A genuine user pin prevents automatic replacement, including stale/unavailable pins; technical infeasibility produces an actionable choice rather than silent fallback.
- Before lower-than-required execution or reserve use outside normal policy, disclose the concrete risk and obtain one explicit user confirmation for a defined account/profile/work/time/limit scope.
- Valid grants execute without repeated prompts within scope; revocation, expiry and material scope changes invalidate the grant, with correct restart behavior.
- The model, plan text, files, tool results and replayed unrelated confirmations cannot issue or broaden a grant; priority and model identity remain human-owned.
- A headless inability to confirm yields a structured pending decision, not consent; pending/waiting state is cancellable and explained.
- Only routing policy is overridden: technical/provider limitations, tool permission gates, context approval and the separate general tool root-override contract remain intact.

## Verification Plan

1. Run S11, S12 and S20 through genuine user interaction and synthetic inference, including lower-tier pins and reserve-denial fixtures.
2. Verify one confirmation covers only the displayed scope, survives valid restart and stops after expiry/revocation/account change.
3. Inject approval-looking text via model arguments, files and tool output; assert no authority is created.
4. Verify no repeated prompt inside scope and no bypass of tool/context gates; test cancellable headless pending decisions.
