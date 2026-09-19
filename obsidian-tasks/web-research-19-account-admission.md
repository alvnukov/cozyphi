---
id: web-research-19-account-admission
title: 19 — Schedule web stages behind interactive work without changing the model
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Background web work uses the shared cross-session/process account admission queue.
    - Interactive requests have admission priority; already running requests are not falsely described as uncharged or preempted.
    - All web stages preserve the configured provider/model/effort/recipient pin under backoff, uncertainty and cancellation.
verification_plan:
    - Run deterministic shared-account admission with two process/session clients, cancellation and backoff.
    - Capture admission order, reservations and effective route for all three model stages.
    - Attach scoped coordinator integration/race results; do not test unrelated providers or whole-repository gates.
created_at: "2026-09-19T19:14:42.085523Z"
updated_at: "2026-09-19T19:14:42.085523Z"
---

## Body

**What to build:** Research yields shared subscription admission to interactive work without changing the chosen web model.

**Blocked by:** [18](web-research-18-research-budgets.md), [priority scheduling](routing-priority-scheduling.md), [user pins](routing-human-exceptions.md).

**Contract:** [Spec](../specs/protected-web-research.md), D2/D13/D16, Q9/Q23/Q37, T28. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Submit each web stage to the existing account coordinator with background class, fixed route identity and research allowance. Revalidate permission/budget when admitted after waiting. Honor shared retry/backoff and cancel queued reservations. Unknown quota must not become an invented numeric token balance. Provider-specific routing prerequisite names do not select that provider for web.

**Do not change:** No second scheduler, opportunistic stronger-model upgrade, fallback route, reserve exception or unlimited queue. If the common API cannot honor the pin, stay blocked and amend its owner contract separately.

**Proof required:** Two sessions/process clients sharing a controlled account: pending interactive work admits before queued background work, cancelled jobs release claims, and captured requests keep the exact binding. Distinguish queue order from already running calls.

**Stop condition:** A local mutex/mock cannot prove cross-process admission; reuse actual shared coordinator tests.

## Acceptance Criteria

- Background web work uses the shared cross-session/process account admission queue.
- Interactive requests have admission priority; already running requests are not falsely described as uncharged or preempted.
- All web stages preserve the configured provider/model/effort/recipient pin under backoff, uncertainty and cancellation.

## Verification Plan

1. Run deterministic shared-account admission with two process/session clients, cancellation and backoff.
2. Capture admission order, reservations and effective route for all three model stages.
3. Attach scoped coordinator integration/race results; do not test unrelated providers or whole-repository gates.
