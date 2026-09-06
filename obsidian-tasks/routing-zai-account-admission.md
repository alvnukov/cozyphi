---
id: routing-zai-account-admission
title: 03 — Route Z.AI Coding Plan through shared account admission
status: blocked
priority: high
model_level: high
task_type: feature
parent_id: subscription-aware-routing
tags:
    - routing
    - subscriptions
    - zai
acceptance_criteria:
    - Existing Z.AI Coding Plan authentication routes assigned work through the same admission interface and cross-process commitments as OpenAI without misclassifying subscription API-key auth as paid API.
    - Adapter observations preserve absent vs zero token/credit fields, native units, account/window identity, reset timestamps and source freshness; malformed/unverified semantics remain unknown.
    - Main selection can choose eligible profiles across the two real provider adapters while preserving independent accounts and genuinely shared windows.
    - Fallback quota responses, external consumption and unavailable telemetry produce conservative necessary execution and clear explanations, never fabricated surplus.
    - Fixture evidence and any separately authorized provider validation are distinguished; undocumented regional/default/overage assumptions cannot silently enable unsafe automatic behavior.
verification_plan:
    - Replay success/fallback/missing/null/zero/malformed Z.AI fixtures and assert unit/presence/reset semantics.
    - Run S04, S08, S09 and S20 for both adapters and distinct/shared account identities.
    - Drive synthetic main selection with OpenAI unavailable and Z.AI eligible; assert required tier and genuine account charging.
    - Publish source/fixture evidence separately from any optional authorized provider observations.
created_at: "2026-09-06T11:10:50.114827Z"
updated_at: "2026-09-06T11:15:04.386885Z"
---

## Body

**Approved slice:** 03 of the [breakdown](../specs/subscription-aware-routing-tickets.md). Read the [contract](../specs/subscription-aware-routing.md), D2–D3.

**Blocked by:** routing-openai-account-admission.

**What to build:** Execute Z.AI Coding Plan work through the same complete account-aware route as OpenAI, proving the quota adapter seam with a second real provider rather than a speculative abstraction.

**Inputs:** Shared admission module and existing Z.AI quota endpoints/decoder behavior.

**Scope/output:** Provider evidence normalization, subscription classification, cross-provider selection, explanatory UI/headless output and contract fixtures. Do not add a new Claude backend or redesign account connection UX.

**Escalation:** Treat uncertain token/credit units, reset formats, missing fields or hidden paid overage as evidence gaps. Live GET/inference verification requires separate user consent.

**Blocked (2026-09-06).** Waiting for routing-openai-account-admission; reuse the working shared account interface for the second adapter.

## Acceptance Criteria

- Existing Z.AI Coding Plan authentication routes assigned work through the same admission interface and cross-process commitments as OpenAI without misclassifying subscription API-key auth as paid API.
- Adapter observations preserve absent vs zero token/credit fields, native units, account/window identity, reset timestamps and source freshness; malformed/unverified semantics remain unknown.
- Main selection can choose eligible profiles across the two real provider adapters while preserving independent accounts and genuinely shared windows.
- Fallback quota responses, external consumption and unavailable telemetry produce conservative necessary execution and clear explanations, never fabricated surplus.
- Fixture evidence and any separately authorized provider validation are distinguished; undocumented regional/default/overage assumptions cannot silently enable unsafe automatic behavior.

## Verification Plan

1. Replay success/fallback/missing/null/zero/malformed Z.AI fixtures and assert unit/presence/reset semantics.
2. Run S04, S08, S09 and S20 for both adapters and distinct/shared account identities.
3. Drive synthetic main selection with OpenAI unavailable and Z.AI eligible; assert required tier and genuine account charging.
4. Publish source/fixture evidence separately from any optional authorized provider observations.
