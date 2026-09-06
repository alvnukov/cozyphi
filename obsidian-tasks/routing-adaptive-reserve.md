---
id: routing-adaptive-reserve
title: 04 — Protect adaptive reserve and spend useful surplus
status: blocked
priority: high
model_level: very_high
task_type: feature
parent_id: subscription-aware-routing
tags:
    - routing
    - subscriptions
acceptance_criteria:
    - Users can configure a cold-start reserve; adaptation uses important-demand history, concurrency, uncertainty and time to each applicable reset with bounded, disclosed parameters.
    - Ordinary work does not automatically consume protected reserve; reserve-only admission asks and waits, important work can use reserve, and confirmed exception grants can be consumed through the later authority path.
    - Reserve releases gradually near reset while retaining long/shared-window protection; claims and expected demand are not double-counted.
    - Credible surplus upgrades already-assigned work at allowed selection events, respects interactive latency and is more permissive for background latency; unknown/stale evidence cannot justify an upgrade.
    - Stable near-equal choices retain the current profile; returning from actual upgrade to required tier is not an unauthorized downgrade; no below-required automatic selection occurs.
    - No additional tasks/retries/output are manufactured to spend quota; decision traces distinguish missed useful upgrades from harmless expiry and expose forecast error.
verification_plan:
    - Run S05–S08, S11 and S18 with controlled time, all-window claims and explicit important demand.
    - Compare required-tier baseline to adaptive policy on fixed workload traces; inspect reserve breaches and missed useful upgrades before utilization.
    - Verify no-work expiry, uncertain resets, external drain and long-window exhaustion do not trigger synthetic spending.
    - Drive reserve settings and upgrade/wait explanations; record all numerical policy parameters and evidence limits.
created_at: "2026-09-06T11:11:11.3491Z"
updated_at: "2026-09-06T11:15:04.38803Z"
---

## Body

**Approved slice:** 04 of the [breakdown](../specs/subscription-aware-routing-tickets.md). Read the [contract](../specs/subscription-aware-routing.md), D4–D5.

**Blocked by:** routing-openai-account-admission.

**What to build:** Protect capacity for explicitly important work while improving already-assigned work with credible expiring surplus, with configurable cold-start behavior and visible reasons. The algorithm is provider-neutral; the second adapter is independently delivered by slice 03.

**Inputs:** Atomic claims, normalized evidence, required-tier baseline and genuine user priority metadata.

**Scope/output:** Reserve/surplus policy through the existing admission interface, settings and decision explanations, fixed-clock traces and bounded calibration. Fair scheduling between sessions comes in slice 05; pins/exception confirmation comes in slice 07.

**Escalation:** Do not claim optimal prediction or exact token headroom from percentages. If a trace requires weakening quality, permission or a long-window reserve, explain the conflict rather than hide it in a score.

**Blocked (2026-09-06).** Waiting for routing-openai-account-admission; policy requires coordinated claims and normalized evidence.

## Acceptance Criteria

- Users can configure a cold-start reserve; adaptation uses important-demand history, concurrency, uncertainty and time to each applicable reset with bounded, disclosed parameters.
- Ordinary work does not automatically consume protected reserve; reserve-only admission asks and waits, important work can use reserve, and confirmed exception grants can be consumed through the later authority path.
- Reserve releases gradually near reset while retaining long/shared-window protection; claims and expected demand are not double-counted.
- Credible surplus upgrades already-assigned work at allowed selection events, respects interactive latency and is more permissive for background latency; unknown/stale evidence cannot justify an upgrade.
- Stable near-equal choices retain the current profile; returning from actual upgrade to required tier is not an unauthorized downgrade; no below-required automatic selection occurs.
- No additional tasks/retries/output are manufactured to spend quota; decision traces distinguish missed useful upgrades from harmless expiry and expose forecast error.

## Verification Plan

1. Run S05–S08, S11 and S18 with controlled time, all-window claims and explicit important demand.
2. Compare required-tier baseline to adaptive policy on fixed workload traces; inspect reserve breaches and missed useful upgrades before utilization.
3. Verify no-work expiry, uncertain resets, external drain and long-window exhaustion do not trigger synthetic spending.
4. Drive reserve settings and upgrade/wait explanations; record all numerical policy parameters and evidence limits.
