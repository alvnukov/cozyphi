---
id: security-durable-provenance
title: 02 — Preserve material restrictions across ingress and session lifecycle
status: blocked
priority: high
model_level: high
task_type: feature
parent_id: harness-security-hardening
tags: [security, provenance]
acceptance_criteria:
  - Every specified ingress produces a host-owned source/version, authority, sensitivity, recipient, verification and derivation envelope.
  - Turns, resume, compaction and child outcomes retain restrictions without trusting model-authored wrappers.
  - Derivatives conservatively inherit accessible input restrictions where exact dependencies are unavailable.
  - Missing or legacy metadata is unknown rather than clean, including after toggling security.
  - A user-visible resumed or compacted flow explains retained restrictions without exposing sensitive content.
verification_plan:
  - Exercise S03 and S04 through real ingress adapters, session round trips, compaction and child outcomes; assert UI/headless explanations preserve restrictions without secret output.
  - Test all ingress families, forged metadata, missing metadata, mixed histories and lossy summaries.
  - Run scoped changed-package tests and serialization compatibility checks; at most one scoped lint.
created_at: "2026-09-09T11:16:39Z"
updated_at: "2026-09-09T11:16:39Z"
---

## Body

**What to build:** A user can resume, compact or delegate a conversation without confidential or hostile material becoming unrestricted. Extend the first protected path across instructions, skills, memory, reads/search/LSP, process output, hooks/watches, MCP, web, user sensitivity and history.

**Blocked by:** security-opt-in-foundation.

**Contract:** [Specification](../specs/harness-security.md), D2–D3; slice 02 of the [approved map](../specs/harness-security-tickets.md).

**Scope:** Admission and durable metadata plus the actual lifecycle adapters and explanations. Do not repurpose delivery identifiers. This slice tracks restrictions; it does not claim the complete outbound/action coverage delivered by 03 and 04. Sensitive raw history remains unavailable before protected storage.

**Blocked (2026-09-09):** Waiting for the policy/material interface and first end-to-end flow.

## Acceptance Criteria

- Every specified ingress produces a host-owned source/version, authority, sensitivity, recipient, verification and derivation envelope.
- Turns, resume, compaction and child outcomes retain restrictions without trusting model-authored wrappers.
- Derivatives conservatively inherit accessible input restrictions where exact dependencies are unavailable.
- Missing or legacy metadata is unknown rather than clean, including after toggling security.
- A user-visible resumed or compacted flow explains retained restrictions without exposing sensitive content.

## Verification Plan

1. Exercise S03 and S04 through real ingress adapters, session round trips, compaction and child outcomes; assert UI/headless explanations preserve restrictions without secret output.
2. Test all ingress families, forged metadata, missing metadata, mixed histories and lossy summaries.
3. Run scoped changed-package tests and serialization compatibility checks; at most one scoped lint.
