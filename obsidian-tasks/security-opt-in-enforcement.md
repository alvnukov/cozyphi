---
id: security-opt-in-enforcement
title: 11 — Enforce reviewed guard policy with quarantine and safe pauses
status: blocked
priority: high
model_level: high
task_type: feature
parent_id: harness-security-hardening
tags: [security, guard, enforcement]
acceptance_criteria:
  - Enforcement is a separate human opt-in and is unavailable until named prerequisites and accepted evaluation thresholds/evidence are recorded.
  - Suspected material is quarantined before ordinary consumption without an automatic sanitized-summary substitute.
  - Required unknown, incomplete or unavailable checks pause; retries and exceptions remain constrained by hard policy.
  - Human decisions are scope/version bound; no handler means stop and a detector never issues a grant.
  - Off cancels pending checks/asks without replaying actions, preserves encrypted storage and leaves legacy protections active.
verification_plan:
  - Exercise S05, S08, S09, S13 and S15 through the real engine with scripted guard outcomes and controlled sinks.
  - Test readiness refusal, replay/stale callbacks, cancellation, missing approval handler and policy-forbidden exceptions.
  - Reuse the versioned eval corpus and run only affected-package tests and one scoped lint; live trials need separate authorization.
created_at: "2026-09-09T11:16:39Z"
updated_at: "2026-09-09T11:16:39Z"
---

## Body

**What to build:** After accepting evaluation evidence, a user explicitly enables enforcement and resolves a quarantined or unavailable-check task through permitted choices, with deterministic policy and ordinary permissions still authoritative.

**Blocked by:** security-confirmed-escalation, security-trusted-profiles, agent-security-adversarial-evals.

**Additional readiness gate:** The user must accept a concrete attack-success threshold, model/version and evaluation evidence before this slice is considered ready. These values are still unresolved, not defaults to invent. Offline scripted integration evidence can demonstrate machinery but does not establish model readiness.

**Contract:** [Specification](../specs/harness-security.md), D1, D6 and D9; slice 11 of the [approved map](../specs/harness-security-tickets.md).

**Scope:** Enforcement opt-in, quarantine resolution, safe retries/exceptions, headless stop and cancellation. Never release unknown content merely because a strong model was unavailable or a queue limit was reached. Evidence for candidate behavior can be collected offline before this runtime mode is offered.

**Blocked (2026-09-09):** Waiting for escalation, trusted roles, eval infrastructure and accepted readiness evidence.

## Acceptance Criteria

- Enforcement is a separate human opt-in and is unavailable until named prerequisites and accepted evaluation thresholds/evidence are recorded.
- Suspected material is quarantined before ordinary consumption without an automatic sanitized-summary substitute.
- Required unknown, incomplete or unavailable checks pause; retries and exceptions remain constrained by hard policy.
- Human decisions are scope/version bound; no handler means stop and a detector never issues a grant.
- Off cancels pending checks/asks without replaying actions, preserves encrypted storage and leaves legacy protections active.

## Verification Plan

1. Exercise S05, S08, S09, S13 and S15 through the real engine with scripted guard outcomes and controlled sinks.
2. Test readiness refusal, replay/stale callbacks, cancellation, missing approval handler and policy-forbidden exceptions.
3. Reuse the versioned eval corpus and run only affected-package tests and one scoped lint; live trials need separate authorization.
