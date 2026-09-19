---
id: web-research-36-format-feasibility
title: 36 — Establish evidence-backed PDF, browser and visual adapter contracts
status: blocked
priority: high
model_level: medium
task_type: docs
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - For PDF, browser and image/OCR, candidate adapters are evaluated against authoritative implementation/docs and actual project platforms.
    - The report specifies supported formats, coordinate mapping, network/FS/process isolation, resource bounds, cancellation, dependency/license and missing evidence.
    - No adapter is marked production-ready from a library README or design alone; unresolved decisions require explicit review before implementation.
verification_plan:
    - Review candidate facts against pinned authoritative sources and the platform/channel inventory.
    - Check every required format has a supported/unavailable decision with explicit missing prerequisites and acceptance owner.
    - Validate document links and fixture provenance; no Go gates for documentation-only output or unapproved live model tests.
created_at: "2026-09-19T19:20:23.744743Z"
updated_at: "2026-09-19T19:20:23.744743Z"
---

## Body

**What to build:** A bounded engineering evidence report that makes later adapter integrations mechanical rather than speculative.

**Blocked by:** [01](web-research-01-channel-inventory.md).

**Contract:** [Spec](../specs/protected-web-research.md), D1/D4/D15, Q5/Q10/Q11/Q16/Q30/Q39, T08/T09/T11/T12/T30. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Inspect proven available implementations and authoritative service/parser/browser contracts before proposing adapters. Compare candidates using a fixed matrix: static PDF versus scans, rendering/read-only actions, visual input, exact regions, public subrequests, credentials, bombs, cancellation and platform primitives. Separate provider protocol capabilities from image/document processing. Identify concrete isolation implementation owners and required reviews; document which choices are settled and which remain unavailable. Use synthetic offline fixtures for a bounded feasibility demonstration only where already safe.

**Do not change:** No production adapters, new cryptography/protocols, real-site browsing, live model quota or selection of a fixed provider/model.

**Proof required:** Versioned sources/citations, candidate matrix, fixture results where executed, and explicit accepted/pending decisions. Unmeasured performance remains unknown.

**Stop condition:** Open architecture/security choices require reviewer acceptance and, where needed, separately sized prerequisite tasks before 37–39 can reopen.

## Acceptance Criteria

- For PDF, browser and image/OCR, candidate adapters are evaluated against authoritative implementation/docs and actual project platforms.
- The report specifies supported formats, coordinate mapping, network/FS/process isolation, resource bounds, cancellation, dependency/license and missing evidence.
- No adapter is marked production-ready from a library README or design alone; unresolved decisions require explicit review before implementation.

## Verification Plan

1. Review candidate facts against pinned authoritative sources and the platform/channel inventory.
2. Check every required format has a supported/unavailable decision with explicit missing prerequisites and acceptance owner.
3. Validate document links and fixture provenance; no Go gates for documentation-only output or unapproved live model tests.
