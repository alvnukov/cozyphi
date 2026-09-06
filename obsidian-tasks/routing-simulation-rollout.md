---
id: routing-simulation-rollout
title: 09 — Validate routing scenarios and publish opt-in readiness
status: blocked
priority: high
model_level: high
task_type: test
parent_id: subscription-aware-routing
tags:
    - routing
    - subscriptions
    - verification
acceptance_criteria:
    - S01–S20 pass through public admission, provider fixture and real engine lifecycle seams for both V1 adapters, including cross-process races and adversarial authority cases.
    - Fixed traces compare required-tier baseline and adaptive policy with all parameters recorded; report quality/authority violations, missed useful upgrades, reserve breaches, latency/waiting, churn and forecast error.
    - Harmless expiry is not failure and synthetic work to spend allowance is forbidden; unfavorable traces are retained rather than cherry-picked.
    - User walkthrough verifies wizard, versioned migration, model awareness, queue/wait decisions, pins, scoped exceptions, privacy-safe explanations and rollback to legacy behavior without artifact reinterpretation.
    - Readiness evidence separates source, fixtures and any separately authorized live observations; unsupported provider assumptions restrict rollout and unresolved authority/quality breaches block release.
    - Publish opt-in user documentation and CHANGELOG for the implemented feature; keep the epic open until all slices are actually delivered.
verification_plan:
    - Run the complete offline S01–S20 matrix with deterministic time/events and public result assertions.
    - Replay identical assigned workloads against baseline/adaptive configurations and publish every outcome with parameters and limitations.
    - Walk through interactive and headless setup, exception, cancellation, recovery and migration flows; inspect actual pre-inference tier records.
    - If separately authorized, collect bounded provider evidence without secrets; never present unrun probes or one-account samples as universal validation.
    - Run changed-package gates and documentation/link checks, then review the feature against the approved contract before release.
created_at: "2026-09-06T11:13:11.808782Z"
updated_at: "2026-09-06T11:15:04.393625Z"
---

## Body

**Approved slice:** 09 of the [breakdown](../specs/subscription-aware-routing-tickets.md). Read the full [contract](../specs/subscription-aware-routing.md), especially S01–S20.

**Blocked by:** routing-zai-account-admission, routing-priority-scheduling, routing-children-lifecycle, routing-human-exceptions, routing-paid-budget.

**What to build:** Prove the complete opt-in user journey and policy behavior across both V1 providers using reproducible offline traces, then publish a truthful readiness report and user guide. This validates already-delivered vertical slices rather than deferring their UI/tests here.

**Inputs:** All eight implementation slices and fixed, adversarial workloads with expected outcomes. executor-quality-evaluation is related methodology, not a blocking source of automatic tier rankings.

**Scope/output:** End-to-end scenario matrix, comparative simulation report, migration/recovery walkthrough, provider evidence limits, release documentation and feature CHANGELOG.

**Escalation:** False authority, unauthorized spend, below-required automatic execution or lost requirements block rollout and return to the owning slice. Live provider/inference probes need a separate user-approved scope.

**Blocked (2026-09-06).** Waiting for routing-zai-account-admission, routing-priority-scheduling, routing-children-lifecycle, routing-human-exceptions and routing-paid-budget before whole-flow readiness verification.

## Acceptance Criteria

- S01–S20 pass through public admission, provider fixture and real engine lifecycle seams for both V1 adapters, including cross-process races and adversarial authority cases.
- Fixed traces compare required-tier baseline and adaptive policy with all parameters recorded; report quality/authority violations, missed useful upgrades, reserve breaches, latency/waiting, churn and forecast error.
- Harmless expiry is not failure and synthetic work to spend allowance is forbidden; unfavorable traces are retained rather than cherry-picked.
- User walkthrough verifies wizard, versioned migration, model awareness, queue/wait decisions, pins, scoped exceptions, privacy-safe explanations and rollback to legacy behavior without artifact reinterpretation.
- Readiness evidence separates source, fixtures and any separately authorized live observations; unsupported provider assumptions restrict rollout and unresolved authority/quality breaches block release.
- Publish opt-in user documentation and CHANGELOG for the implemented feature; keep the epic open until all slices are actually delivered.

## Verification Plan

1. Run the complete offline S01–S20 matrix with deterministic time/events and public result assertions.
2. Replay identical assigned workloads against baseline/adaptive configurations and publish every outcome with parameters and limitations.
3. Walk through interactive and headless setup, exception, cancellation, recovery and migration flows; inspect actual pre-inference tier records.
4. If separately authorized, collect bounded provider evidence without secrets; never present unrun probes or one-account samples as universal validation.
5. Run changed-package gates and documentation/link checks, then review the feature against the approved contract before release.
