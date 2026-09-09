---
id: security-v1-readiness
title: 12 — Validate the complete security journey and publish V1 limits
status: blocked
priority: high
model_level: high
task_type: test
parent_id: harness-security-hardening
tags: [security, evals, rollout]
acceptance_criteria:
  - The off/observe/enforce walkthrough covers trust setup, encrypted resume, untrusted transitions, headless stop and rollback without secret output.
  - The report separates actual leak/action outcomes, detector labels and observe potential stops, including repeated attempts and uncertainty.
  - Benign task false stops target at most five percent; ordinary/strong p95 targets are one/five seconds including queue/network but excluding human time.
  - Model/version, corpus, sample sizes, costs, questions, escalation rate and uncovered material are reported without unsupported guarantees.
  - Unmet readiness decisions remain explicit blockers; V1 process-egress limits and the next-stage design are disclosed.
verification_plan:
  - Exercise S01–S17 with offline synthetic data and controlled sinks through the completed public workflow.
  - Run separately authorized model trials only if permission exists; otherwise publish the missing-evidence blocker rather than fabricate readiness.
  - Run only changed-scope checks locally and review the user guide against the accepted evidence and actual CI results.
created_at: "2026-09-09T11:16:39Z"
updated_at: "2026-09-09T11:16:39Z"
---

## Body

**What to build:** A user and reviewer can follow the complete V1 journey and decide whether measured evidence supports offering enforcement. This is whole-flow verification, not the first tests or explanations; every earlier slice supplies its own.

**Blocked by:** security-opt-in-enforcement.

**Contract:** [Specification](../specs/harness-security.md), D9 and testing matrix; slice 12 of the [approved map](../specs/harness-security-tickets.md).

**Scope:** Reproducible report, comparison of modes, resource/latency/cost evidence, rollout and rollback guide. No monetary budget or guard model is predetermined. Historical small-model scores and offline fake providers do not prove system protection. Process isolation is not a V1 delivery claim or a prerequisite for honestly reporting its absence.

**Blocked (2026-09-09):** Waiting for opt-in enforcement and its accepted prerequisites.

## Acceptance Criteria

- The off/observe/enforce walkthrough covers trust setup, encrypted resume, untrusted transitions, headless stop and rollback without secret output.
- The report separates actual leak/action outcomes, detector labels and observe potential stops, including repeated attempts and uncertainty.
- Benign task false stops target at most five percent; ordinary/strong p95 targets are one/five seconds including queue/network but excluding human time.
- Model/version, corpus, sample sizes, costs, questions, escalation rate and uncovered material are reported without unsupported guarantees.
- Unmet readiness decisions remain explicit blockers; V1 process-egress limits and the next-stage design are disclosed.

## Verification Plan

1. Exercise S01–S17 with offline synthetic data and controlled sinks through the completed public workflow.
2. Run separately authorized model trials only if permission exists; otherwise publish the missing-evidence blocker rather than fabricate readiness.
3. Run only changed-scope checks locally and review the user guide against the accepted evidence and actual CI results.
