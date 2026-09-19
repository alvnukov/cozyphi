---
id: web-research-39-visual-sources
title: 39 — Interpret images and scans inside the configured visual quarantine
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Configured visual-capable web model covers the actual image/scan regions consumed; OCR alone is never claimed sufficient.
    - Visual input stays inside quarantine and only checked textual interpretation plus references reaches the parent.
    - Image/OCR workers are bounded and isolated; unsupported model/adapter/platform refuses without alternate-model fallback.
verification_plan:
    - Run synthetic diagram, scanned-page, mixed visual/text and visual-only injection fixtures through public research.
    - Capture effective provider messages and parent transcript; verify consumed pixels reach only web quarantine and all output text is screened.
    - Test unsupported vision, decode/resource failure and cancellation with real worker boundaries; attach scoped adapter results.
created_at: "2026-09-19T19:20:23.999474Z"
updated_at: "2026-09-19T19:20:23.999474Z"
---

## Body

**What to build:** Answer a question about a diagram/image or scanned page using the already chosen web model and reviewed visual adapter.

**Blocked by:** [04](web-research-04-consented-preflight.md), [16](web-research-16-consumed-coverage.md), [36](web-research-36-format-feasibility.md).

**Unresolved prerequisite:** Reviewed image/OCR representation/coverage contract, proven provider visual transport and landed worker isolation per platform; link concrete implementation proofs before reopening.

**Contract:** [Spec](../specs/protected-web-research.md), D2/D4/D5/D6/D15, Q5/Q9/Q11/Q16/Q29/Q39, T01/T08/T09/T12/T30. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Feed authorized visual regions into the separate safety/extraction contexts using the same pinned model. OCR may assist but cannot replace omitted visual coverage. Bind page/region references to immutable material, final-screen generated text, and keep image pixels out of parent/tool transcripts. Bound decoding, resolution, region count and worker lifetime using the approved adapter contract.

**Do not change:** No secondary vision model, guessed provider payload, pixels to parent, unsupported format success or blanket image-safety claim.

**Proof required:** Diagram and scanned-text fixtures including visual-only injection; capture both quarantine inputs and parent output to prove actual visual coverage and text-only release. Missing vision and decoder bombs refuse.

**Stop condition:** Unknown visual protocol/capability requires authoritative evidence, not inference from a model name.

## Acceptance Criteria

- Configured visual-capable web model covers the actual image/scan regions consumed; OCR alone is never claimed sufficient.
- Visual input stays inside quarantine and only checked textual interpretation plus references reaches the parent.
- Image/OCR workers are bounded and isolated; unsupported model/adapter/platform refuses without alternate-model fallback.

## Verification Plan

1. Run synthetic diagram, scanned-page, mixed visual/text and visual-only injection fixtures through public research.
2. Capture effective provider messages and parent transcript; verify consumed pixels reach only web quarantine and all output text is screened.
3. Test unsupported vision, decode/resource failure and cancellation with real worker boundaries; attach scoped adapter results.
