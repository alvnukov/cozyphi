---
id: web-research-32-circuit-breaker
title: 32 — Pause systematic web-model violations without erasing incidents
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Repeated independent-source violations can pause the affected web configuration before continued mass blocking.
    - Pause retains existing incidents/blocks and stops new work without automatic clearing or model substitution.
    - Thresholds are explicit configurable inputs pending measured defaults, with safe reason/status and user-controlled diagnosis/continuation.
verification_plan:
    - Use explicit test thresholds and fake clock with independent, repeated and duplicate source events.
    - Assert pause stops new admission and cannot be bypassed by a new job ID or model retry.
    - Record counters, retained blocks and user diagnostic transition; run scoped shared-state tests.
created_at: "2026-09-19T19:18:45.126549Z"
updated_at: "2026-09-19T19:18:45.126549Z"
---

## Body

**What to build:** A bounded circuit breaker for systematic model/configuration failures, distinct from source-specific quarantine.

**Blocked by:** [18](web-research-18-research-budgets.md), [27](web-research-27-host-blocks.md).

**Contract:** [Spec](../specs/protected-web-research.md), D9/D13, Q33/Q37, T21/T28. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Aggregate bounded safe incident facts by effective configuration and independent source identity using the shared state owner. Apply an explicitly configured count/window policy with test-only values; empirical defaults belong to 45. On threshold crossing pause further admissions and reconcile in-flight work without erasing earlier evidence. Expose a safe diagnostic state; ordinary technical failures and duplicate events must not be silently counted as independent malicious sites.

**Do not change:** No automatic unblock, infinite model-triggered retest, fallback model or invented threshold claimed optimal.

**Proof required:** Controlled incident sequence across sources and duplicate callbacks showing exact transition to paused and zero further admissions. Existing blocks persist; a permitted user diagnostic action is distinguishable from resumed normal research.

**Stop condition:** If aggregate state cannot coordinate across active processes, record that shared-owner gap instead of using a per-widget counter.

## Acceptance Criteria

- Repeated independent-source violations can pause the affected web configuration before continued mass blocking.
- Pause retains existing incidents/blocks and stops new work without automatic clearing or model substitution.
- Thresholds are explicit configurable inputs pending measured defaults, with safe reason/status and user-controlled diagnosis/continuation.

## Verification Plan

1. Use explicit test thresholds and fake clock with independent, repeated and duplicate source events.
2. Assert pause stops new admission and cannot be bypassed by a new job ID or model retry.
3. Record counters, retained blocks and user diagnostic transition; run scoped shared-state tests.
