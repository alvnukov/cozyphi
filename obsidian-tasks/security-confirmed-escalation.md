---
id: security-confirmed-escalation
title: 06 — Confirm strong-model escalation without granting authority
status: blocked
priority: high
model_level: high
task_type: feature
parent_id: harness-security-hardening
tags: [security, guard, approval]
acceptance_criteria:
  - Every strong-model escalation requires distinct explicit user confirmation of recipient and permitted data.
  - No original secrets reach an external escalation; a redacted-copy verdict does not clear the original.
  - Refusal, missing approval UI and unavailable mandatory checking never become consent or safe verdicts.
  - Escalation follows bounded dispatch, cancellation and audit contracts without recursive guards.
  - UI/headless output reports uncertainty and permitted next steps without claiming an agreed budget or measured latency.
verification_plan:
  - Exercise S09 with a controlled strong provider and assert zero requests before confirmation or after refusal.
  - Test secret-bearing originals, partial redaction, changed recipient, timeout, off transition and stale callbacks.
  - Run only changed-package tests and at most one scoped lint; no live paid calls without separate approval.
created_at: "2026-09-09T11:16:39Z"
updated_at: "2026-09-09T11:16:39Z"
---

## Body

**What to build:** A user can explicitly request a stronger second opinion on permitted data, see its limitations and decline without a hidden call. Technical limits apply even though no monetary budget has been chosen.

**Blocked by:** security-observe-guard.

**Contract:** [Specification](../specs/harness-security.md), D6 and D9; slice 06 of the [approved map](../specs/harness-security-tickets.md).

**Scope:** Human confirmation, bounded escalation, policy-constrained input and structured outcome. A stronger verdict is still a signal, never a grant or automatic declassification. The five-second p95 target includes queue/network, excludes human waiting and is not a promise.

**Blocked (2026-09-09):** Waiting for isolated guard contracts and safe observation.

## Acceptance Criteria

- Every strong-model escalation requires distinct explicit user confirmation of recipient and permitted data.
- No original secrets reach an external escalation; a redacted-copy verdict does not clear the original.
- Refusal, missing approval UI and unavailable mandatory checking never become consent or safe verdicts.
- Escalation follows bounded dispatch, cancellation and audit contracts without recursive guards.
- UI/headless output reports uncertainty and permitted next steps without claiming an agreed budget or measured latency.

## Verification Plan

1. Exercise S09 with a controlled strong provider and assert zero requests before confirmation or after refusal.
2. Test secret-bearing originals, partial redaction, changed recipient, timeout, off transition and stale callbacks.
3. Run only changed-package tests and at most one scoped lint; no live paid calls without separate approval.
