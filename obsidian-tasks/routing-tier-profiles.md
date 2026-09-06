---
id: routing-tier-profiles
title: 01 — Configure versioned quality-tier profiles and migrate effort
status: blocked
priority: high
model_level: high
task_type: feature
parent_id: subscription-aware-routing
tags:
    - routing
    - subscriptions
acceptance_criteria:
    - A wizard explicitly enables routing and persists participation plus exactly one low/medium/high/xhigh tier per verified model/native-effort profile; new models remain unranked and excluded.
    - 'Plan/spawn effort interpretation is artifact-versioned: new mode requests tiers, empty inherits, none/minimal/max reject rather than coerce; legacy sessions/plans/jobs are unchanged until a previewed, confirmed migration.'
    - Effective options, variants and actual wire effort agree with the profile; unknown/ignored native effort cannot masquerade as an explicit verified setting.
    - A controlled main/child launch demonstrates default required-tier inheritance and authoritative required/actual/native-effort metadata before inference; an upgraded parent does not raise inherited requirements.
    - UI and headless diagnostics disclose active mode, profile and pending/effective state; task model_level vocabulary is not silently mapped to tiers.
verification_plan:
    - Exercise S01–S03 with legacy/new/mixed-mode serialized artifacts, restart and rejected migrations.
    - Capture synthetic provider requests for explicit effort, options precedence, provider default and ignored/unsupported settings.
    - Drive the wizard and a controlled main/child launch; assert authoritative metadata precedes inference and no account-aware routing is implied.
    - Run scoped schema, persistence, prompt and selection regressions.
created_at: "2026-09-06T11:10:08.596245Z"
updated_at: "2026-09-06T11:15:04.384146Z"
---

## Body

**Approved slice:** 01 of the [breakdown](../specs/subscription-aware-routing-tickets.md). Read the [contract](../specs/subscription-aware-routing.md), D1 and D7.

**Blocked by:** model-specific-openai-codex-efforts.

**What to build:** Let a user enable the new versioned mode, configure trustworthy quality profiles, and run a controlled profile through main/child execution with truthful tier metadata. This is the complete profile/configuration path, not account-aware automatic selection yet.

**Inputs:** Correct model-specific capabilities and existing human-owned selection/prompt paths.

**Scope/output:** Wizard, persisted profile/mode contracts, explicit artifact migration and mode-aware plan/spawn validation, visible state, wire/profile tests and compatibility fixtures.

**Escalation:** Unverifiable default effort or adapter behavior must exclude the affected explicit profile, not invent a mapping. Preserve existing executor context and approval requirements.

**Blocked (2026-09-06).** Waiting for model-specific-openai-codex-efforts; the wizard must not rank profiles against an inaccurate effort catalog.

## Acceptance Criteria

- A wizard explicitly enables routing and persists participation plus exactly one low/medium/high/xhigh tier per verified model/native-effort profile; new models remain unranked and excluded.
- Plan/spawn effort interpretation is artifact-versioned: new mode requests tiers, empty inherits, none/minimal/max reject rather than coerce; legacy sessions/plans/jobs are unchanged until a previewed, confirmed migration.
- Effective options, variants and actual wire effort agree with the profile; unknown/ignored native effort cannot masquerade as an explicit verified setting.
- A controlled main/child launch demonstrates default required-tier inheritance and authoritative required/actual/native-effort metadata before inference; an upgraded parent does not raise inherited requirements.
- UI and headless diagnostics disclose active mode, profile and pending/effective state; task model_level vocabulary is not silently mapped to tiers.

## Verification Plan

1. Exercise S01–S03 with legacy/new/mixed-mode serialized artifacts, restart and rejected migrations.
2. Capture synthetic provider requests for explicit effort, options precedence, provider default and ignored/unsupported settings.
3. Drive the wizard and a controlled main/child launch; assert authoritative metadata precedes inference and no account-aware routing is implied.
4. Run scoped schema, persistence, prompt and selection regressions.
