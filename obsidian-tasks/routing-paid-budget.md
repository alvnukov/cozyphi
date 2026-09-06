---
id: routing-paid-budget
title: 08 — Gate paid routing with explicit scope and monetary bounds
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
    - Paid routes remain excluded until explicit user opt-in defines monetary ceiling and scope; API-key authentication alone is not evidence of paid billing.
    - Ordinary automatic paid admission requires trustworthy price data and an enforceable conservative call-cost bound, reserved atomically across all processes covered by the grant.
    - Accounting includes retry, compaction, failed/cancelled/uncertain calls and output bounds; post-hoc usage is not presented as a hard cap.
    - Missing/stale prices, unbounded costs and exhausted cap exclude ordinary automatic paid routing and yield an actionable decision without silent fallback.
    - A separate genuine once-confirmed scope may authorize the disclosed cap or unknown-cost exception; a pin or prior paid opt-in does not imply that exception.
    - UI/headless explanations distinguish ceiling, commitments, observed cost, uncertainty and exceptions without credentials; no provider-charge reversal guarantee is claimed.
verification_plan:
    - Run S13–S15 and S20 with fake prices/transports and two local processes racing for the final monetary allowance.
    - Cover price changes, unknown costs, maximum output, failed/cancelled/retried calls and uncertain charges.
    - Verify disabled-by-default routes never dispatch, then demonstrate bounded opt-in and separately confirmed cap exception.
    - Inspect persisted scopes and explanations for revocation, restart, account switch and secret leakage; no live spending is required.
created_at: "2026-09-06T11:12:43.372011Z"
updated_at: "2026-09-06T11:15:04.392652Z"
---

## Body

**Approved slice:** 08 of the [breakdown](../specs/subscription-aware-routing-tickets.md). Read the [contract](../specs/subscription-aware-routing.md), D6.

**Blocked by:** routing-human-exceptions.

**What to build:** Offer explicit controlled paid fallback for existing configured inference routes, off by default, with genuine monetary admission rather than a retrospective spend display.

**Inputs:** Account reservations and scoped human exception lifecycle, transitively supplied by slices 02 and 07.

**Scope/output:** Paid opt-in controls, price/bound eligibility, atomic monetary reservations/reconciliation, separately confirmed cap exceptions and visible explanations. This is not a new subscription backend.

**Escalation:** If a route cannot support a reliable upper bound, exclude ordinary auto spending. Do not label an estimate a hard monetary guarantee; any user exception must disclose the remaining uncertainty.

**Blocked (2026-09-06).** Waiting for routing-human-exceptions; paid opt-in and cap exceptions require genuine scoped authority.

## Acceptance Criteria

- Paid routes remain excluded until explicit user opt-in defines monetary ceiling and scope; API-key authentication alone is not evidence of paid billing.
- Ordinary automatic paid admission requires trustworthy price data and an enforceable conservative call-cost bound, reserved atomically across all processes covered by the grant.
- Accounting includes retry, compaction, failed/cancelled/uncertain calls and output bounds; post-hoc usage is not presented as a hard cap.
- Missing/stale prices, unbounded costs and exhausted cap exclude ordinary automatic paid routing and yield an actionable decision without silent fallback.
- A separate genuine once-confirmed scope may authorize the disclosed cap or unknown-cost exception; a pin or prior paid opt-in does not imply that exception.
- UI/headless explanations distinguish ceiling, commitments, observed cost, uncertainty and exceptions without credentials; no provider-charge reversal guarantee is claimed.

## Verification Plan

1. Run S13–S15 and S20 with fake prices/transports and two local processes racing for the final monetary allowance.
2. Cover price changes, unknown costs, maximum output, failed/cancelled/retried calls and uncertain charges.
3. Verify disabled-by-default routes never dispatch, then demonstrate bounded opt-in and separately confirmed cap exception.
4. Inspect persisted scopes and explanations for revocation, restart, account switch and secret leakage; no live spending is required.
