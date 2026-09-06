---
id: routing-openai-account-admission
title: 02 — Admit OpenAI work against shared account commitments
status: blocked
priority: high
model_level: very_high
task_type: feature
parent_id: subscription-aware-routing
tags:
    - routing
    - subscriptions
    - openai
acceptance_criteria:
    - An unpinned main session selects a participating technically feasible OpenAI OAuth/Codex profile meeting required tier, with a visible decision reason; API-key authentication is not misclassified as subscription quota.
    - All local processes/workspaces sharing an account coordinate atomic consumption reservations across every applicable window; aliases and token refresh do not create independent balances.
    - Missing/stale/zero evidence, reset horizons and percentage units remain distinct; necessary execution is possible under unknown quota, but unknown data never establishes surplus.
    - Every chargeable attempt is reserved/reconciled idempotently, including compaction, retry, failure and cancellation; observation watermarks prevent double subtraction or stale credit resurrection.
    - Process death, uncertain charges, coordination failure and reset rollover produce bounded recovery without double refund or uncoordinated admission; secrets and other assignment content stay private.
    - Public admit/wait/ask/unavailable outcomes and UI/headless explanations expose the minimum-tier baseline; adaptive upgrades and generalized child rerouting remain later slices.
verification_plan:
    - Exercise S04, S08, S09, S15 and S20 using two real local test processes, a fake clock and synthetic provider transport.
    - Test alias/refresh/account-switch identity, simultaneous claims, stale/out-of-order snapshots and duplicate completion across resets.
    - Capture main/compaction/retry/cancelled attempts; verify conservative reconciliation and no leaked credentials.
    - Demonstrate required-tier selection and structured wait/unavailable results in main UI and headless output without live spending.
created_at: "2026-09-06T11:10:29.053288Z"
updated_at: "2026-09-06T11:15:04.385788Z"
---

## Body

**Approved slice:** 02 of the [breakdown](../specs/subscription-aware-routing-tickets.md). Read the [contract](../specs/subscription-aware-routing.md), D2–D4.

**Blocked by:** routing-tier-profiles.

**What to build:** Run ordinary main work through a shared account admission interface using the existing OpenAI OAuth/Codex quota adapter, demonstrating two local processes cannot independently promise the same allowance. Keep the initial policy at required-quality execution; no burn-before-reset optimization yet.

**Inputs:** Versioned profiles and trustworthy effective effort. Existing quota observations are evidence, not a financial ledger.

**Scope/output:** Stable private account identity, normalized evidence, atomic cross-process claims and reconciliation, main inference integration, cancellation/recovery and visible admission outcomes.

**Escalation:** Percentage-to-token precision, external-app locking and prevention of provider-estimate overshoot cannot be promised. Unknown identity cannot create a fresh allowance; unavailable coordination must be actionable, not bypassed.

**Blocked (2026-09-06).** Waiting for routing-tier-profiles; account admission consumes its versioned profile and required-tier contract.

## Acceptance Criteria

- An unpinned main session selects a participating technically feasible OpenAI OAuth/Codex profile meeting required tier, with a visible decision reason; API-key authentication is not misclassified as subscription quota.
- All local processes/workspaces sharing an account coordinate atomic consumption reservations across every applicable window; aliases and token refresh do not create independent balances.
- Missing/stale/zero evidence, reset horizons and percentage units remain distinct; necessary execution is possible under unknown quota, but unknown data never establishes surplus.
- Every chargeable attempt is reserved/reconciled idempotently, including compaction, retry, failure and cancellation; observation watermarks prevent double subtraction or stale credit resurrection.
- Process death, uncertain charges, coordination failure and reset rollover produce bounded recovery without double refund or uncoordinated admission; secrets and other assignment content stay private.
- Public admit/wait/ask/unavailable outcomes and UI/headless explanations expose the minimum-tier baseline; adaptive upgrades and generalized child rerouting remain later slices.

## Verification Plan

1. Exercise S04, S08, S09, S15 and S20 using two real local test processes, a fake clock and synthetic provider transport.
2. Test alias/refresh/account-switch identity, simultaneous claims, stale/out-of-order snapshots and duplicate completion across resets.
3. Capture main/compaction/retry/cancelled attempts; verify conservative reconciliation and no leaked credentials.
4. Demonstrate required-tier selection and structured wait/unavailable results in main UI and headless output without live spending.
