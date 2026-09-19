---
id: web-research-11-static-answer
title: 11 — Assemble one static-source answer without enabling incomplete protection
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - A controlled public-session scenario for one static source produces a checked answer with immutable source/version/time.
    - The assembled runtime code includes required initial/final checks, exact citations, provenance and recipient gates, but this ticket does not enable production research.
    - Production remains fail-closed until aggregate gate 41 verifies mandatory integrations; offline adapter success is not readiness.
verification_plan:
    - Run controlled public-session text, HTML code/table and malicious-metadata cases through the assembled route.
    - Invoke the ordinary production entry without aggregate readiness and assert zero production fetch/model delivery even when the single-source test passes.
    - Attach actual core isolation/provenance prerequisite proofs, controlled source/candidate identities and scoped results; no live trial or production-enable in this ticket.
created_at: "2026-09-19T19:13:21.022125Z"
updated_at: "2026-09-19T19:32:43.919321Z"
---

## Body

**What to build:** Assemble the verified acquisition route and single-source tracer for a static-source workflow, proven offline with controlled network/provider adapters. Completion means integration ready for further tickets, not enabled production web.

**Blocked by:** [04](web-research-04-consented-preflight.md), [08](web-research-08-release-races.md), [09](web-research-09-exact-citations.md), [10](web-research-10-public-acquisition.md), [durable provenance](security-durable-provenance.md).

**Unresolved prerequisite:** Actual core isolation/disabled-channel proof identified by inventory 01 under the shared isolation contract; record landed implementation IDs before reopening. A design note is insufficient.

**Contract:** [Spec](../specs/protected-web-research.md), D3–D8/D15, Q3/Q13/Q17/Q18/Q25/Q26, T02/T09/T10/T13/T14/T30. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Join bounded fetching, existing normalization and immutable snapshot registration. Preserve code/tables, provenance and consumed coverage. Register released test material with shared provenance. Keep persistence/search/extra formats out. Wire the runtime route behind a host-owned aggregate eligibility check that remains false until 41 verifies delivered budgets/account scheduling, lifecycle/action integration, block/revocation and human resolution. Do not add 41 as a dependency here: later integration builds on this disabled path. Missing future capability is expected, not a reason for a default-true flag. Production readiness and rollout are distinct from test injection and this task's completion.

**Do not change:** No production-enable, raw fallback, parser stack or full-feature claim.

**Proof required:** Controlled question-to-answer trace with request capture, normalized fixture, exact citation and restricted output. Ordinary configured production calls still refuse before aggregate readiness; missing isolation and hostile titles release no unchecked bytes.

**Stop condition:** Missing core isolation/provenance/coverage blocks integration; missing later mandatory features keeps production eligibility false, without substituting offline adapters.

## Acceptance Criteria

- A controlled public-session scenario for one static source produces a checked answer with immutable source/version/time.
- The assembled runtime code includes required initial/final checks, exact citations, provenance and recipient gates, but this ticket does not enable production research.
- Production remains fail-closed until aggregate gate 41 verifies mandatory integrations; offline adapter success is not readiness.

## Verification Plan

1. Run controlled public-session text, HTML code/table and malicious-metadata cases through the assembled route.
2. Invoke the ordinary production entry without aggregate readiness and assert zero production fetch/model delivery even when the single-source test passes.
3. Attach actual core isolation/provenance prerequisite proofs, controlled source/candidate identities and scoped results; no live trial or production-enable in this ticket.
