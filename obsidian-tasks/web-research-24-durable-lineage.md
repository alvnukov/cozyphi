---
id: web-research-24-durable-lineage
title: 24 — Preserve web derivation restrictions through session lifecycle
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Admitted web material and conservative derivatives retain host-owned provenance across user/autonomous turns, resume, fork, compaction and children.
    - Missing lineage remains restricted; disabling web or general security never cleans existing content.
    - The integration uses the shared provenance model and does not create a second taint store.
verification_plan:
    - Use public turn/resume/fork/compact/delegation operations with synthetic restricted material.
    - Check actual next-request/action admission rather than only serialized flags; include missing-metadata and mode-toggle cases.
    - Run changed-package lifecycle tests and record each operation's provenance and enforcement evidence.
created_at: "2026-09-19T19:15:57.796816Z"
updated_at: "2026-09-19T19:15:57.796816Z"
---

## Body

**What to build:** Connect web source/result identities to the delivered shared provenance lifecycle rather than resetting turn-local taint.

**Blocked by:** [11](web-research-11-static-answer.md), [durable provenance](security-durable-provenance.md).

**Contract:** [Spec](../specs/protected-web-research.md), D7/D16, Q1/Q14/Q25/Q34, T02/T20. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Register source, consumed coverage, candidate and released result derivations using shared primitives. Reuse existing lifecycle propagation; add web-specific integration tests for real user input, autonomous wake, resume, fork, compaction and child outcome. Unknown reductions conservatively inherit dependencies. Fork support must be explicit in the shared contract before this slice can complete; extend that owner separately if absent.

**Do not change:** No broad provenance redesign, model-written trust labels, warning-as-cleanup or clearance on mode changes.

**Proof required:** For a synthetic restricted source, inspect effective subsequent model/action admission after each lifecycle operation and after dropping metadata. Restrictions remain; a clean independent source is the control. Persisted metadata alone is not sufficient proof.

**Stop condition:** If shared lifecycle primitives are missing, block with exact missing operation; do not rebuild them inside web.

## Acceptance Criteria

- Admitted web material and conservative derivatives retain host-owned provenance across user/autonomous turns, resume, fork, compaction and children.
- Missing lineage remains restricted; disabling web or general security never cleans existing content.
- The integration uses the shared provenance model and does not create a second taint store.

## Verification Plan

1. Use public turn/resume/fork/compact/delegation operations with synthetic restricted material.
2. Check actual next-request/action admission rather than only serialized flags; include missing-metadata and mode-toggle cases.
3. Run changed-package lifecycle tests and record each operation's provenance and enforcement evidence.
