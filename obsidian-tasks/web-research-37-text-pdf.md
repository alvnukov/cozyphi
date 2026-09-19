---
id: web-research-37-text-pdf
title: 37 — Answer text-PDF questions through a proven isolated adapter
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - An authorized text-PDF question returns checked content with reproducible page/region provenance.
    - Parsing runs under the approved bounded isolated adapter; malformed/bomb/cancel inputs cannot escape or run indefinitely.
    - Scanned/mixed visual content is not declared understood by text extraction alone; unavailable visual support is explicit.
verification_plan:
    - Submit synthetic text/table PDFs and verify selected-page inputs, citations and final checked output.
    - Exercise malformed/bomb/cancel cases at actual worker boundaries and inspect forbidden sinks/descendant cleanup.
    - Run only affected PDF/web adapter integration tests on supported platforms; record unavailable platforms honestly.
created_at: "2026-09-19T19:20:23.830031Z"
updated_at: "2026-09-19T19:20:23.830031Z"
---

## Body

**What to build:** One end-to-end text-PDF research path using a reviewed existing parser and the common coverage/release pipeline.

**Blocked by:** [16](web-research-16-consumed-coverage.md), [36](web-research-36-format-feasibility.md).

**Unresolved prerequisite:** Approved concrete PDF adapter contract and landed platform process/FS/network/resource isolation with evidence, not just security-process-isolation-design. Record implementation task IDs before reopening.

**Contract:** [Spec](../specs/protected-web-research.md), D4/D5/D15, Q5/Q11/Q29/Q39, T08/T09/T12/T30. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Acquire through the common public route, launch the existing isolated parser, map normalized text/page positions into immutable coverage and answer a selected-page question. Preserve tables/code as supported and disclose normalization loss. Stop workers and descendants on resource/cancel limits. Route scanned/mixed pages to unavailable or later visual handling, never an OCR-only completeness claim.

**Do not change:** No homemade PDF parser, general process sandbox, whole-document pass or unsafe temp-file fallback.

**Proof required:** Benign text/table PDF plus malformed, oversize/decompression and cancellation fixtures; exact source regions and forbidden process/FS/network sinks. Unsupported platform refuses safely.

**Stop condition:** If adapter contract/isolation is absent, leave this task blocked rather than design it inside the integration.

## Acceptance Criteria

- An authorized text-PDF question returns checked content with reproducible page/region provenance.
- Parsing runs under the approved bounded isolated adapter; malformed/bomb/cancel inputs cannot escape or run indefinitely.
- Scanned/mixed visual content is not declared understood by text extraction alone; unavailable visual support is explicit.

## Verification Plan

1. Submit synthetic text/table PDFs and verify selected-page inputs, citations and final checked output.
2. Exercise malformed/bomb/cancel cases at actual worker boundaries and inspect forbidden sinks/descendant cleanup.
3. Run only affected PDF/web adapter integration tests on supported platforms; record unavailable platforms honestly.
