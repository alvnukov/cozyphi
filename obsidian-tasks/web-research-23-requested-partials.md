---
id: web-research-23-requested-partials
title: 23 — Retrieve checked partial results only on explicit request
status: blocked
priority: medium
model_level: low
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Only an explicit caller request returns an already checked partial result.
    - Partial retrieval reuses the normal final screening/revocation/ownership boundary and identifies missing coverage.
    - No request causes intermediate token streaming, duplicate automatic final delivery or reset/bypass of the common research allowance.
verification_plan:
    - Pause one source and complete another; inspect default delivery before explicit retrieval.
    - Request partial under valid, revoked and insufficient shared-budget conditions; capture actual allowance accounting.
    - Compare screened/emitted content and final delivery count; run scoped public-operation tests.
created_at: "2026-09-19T19:15:57.712342Z"
updated_at: "2026-09-19T19:32:44.006507Z"
---

## Body

**What to build:** Retrieve useful checked work while research is incomplete, without unsolicited fragments.

**Blocked by:** [17](web-research-17-independent-partials.md), [18](web-research-18-research-budgets.md), [21](web-research-21-origin-delivery.md).

**Contract:** [Spec](../specs/protected-web-research.md), D6/D12/D14, Q12/Q24, T06/T26/T29. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Wire the existing partial candidate/release path to an explicit bounded result request. Snapshot eligible independent evidence, revalidate ownership/source/configuration and final-screen the exact candidate. Charge retrieval's required model work against the common allowance from 18, not local counters. If nothing is releasable or required checking cannot fit, return safe pending/stopped status. Preserve one default final delivery. Production remains behind aggregate readiness.

**Do not change:** No weaker partial pipeline, polling loop, automatic stream, reset budget or complete-research claim.

**Proof required:** One source finished and one pending emits no content until requested. Requested partial is checked and labelled; later final delivery is not duplicated. Revoked or insufficient-check-budget partials refuse without unchecked text.

**Stop condition:** Ambiguous dependencies exclude that evidence rather than weaken the barrier.

## Acceptance Criteria

- Only an explicit caller request returns an already checked partial result.
- Partial retrieval reuses the normal final screening/revocation/ownership boundary and identifies missing coverage.
- No request causes intermediate token streaming, duplicate automatic final delivery or reset/bypass of the common research allowance.

## Verification Plan

1. Pause one source and complete another; inspect default delivery before explicit retrieval.
2. Request partial under valid, revoked and insufficient shared-budget conditions; capture actual allowance accounting.
3. Compare screened/emitted content and final delivery count; run scoped public-operation tests.
