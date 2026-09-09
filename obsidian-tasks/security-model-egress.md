---
id: security-model-egress
title: 03 — Mediate every effective model request and recipient
status: blocked
priority: high
model_level: high
task_type: feature
parent_id: harness-security-hardening
tags: [security, llm, exfiltration]
acceptance_criteria:
  - Main, child, reader, guard, compactor and summarizer dispatches share effective-payload recipient enforcement.
  - System/history/tools/attachments and provider-side state are covered rather than only the last message.
  - Every retry revalidates authority; changed endpoints, redirects and fallbacks require an applicable recipient decision before transmission.
  - Unknown retained remote context is reset or refused rather than treated as clean.
  - Denied or unavailable dispatches explain the actual boundary in UI/headless output without secrets.
verification_plan:
  - Exercise S07 and S14 using captured fake-provider requests across every role and transport attempt.
  - Test redirects, queue-time revocation, fallback, retained remote context, cancellation and missing metadata.
  - Before provider changes, verify authoritative service semantics and a reference implementation; run only scoped adapter/engine checks.
created_at: "2026-09-09T11:16:39Z"
updated_at: "2026-09-09T11:16:39Z"
---

## Body

**What to build:** An intended model recipient receives only permitted context regardless of which role, retry or compaction path sends it. Demonstrate both a permitted request and a blocked recipient change at the transport boundary.

**Blocked by:** security-durable-provenance.

**Contract:** [Specification](../specs/harness-security.md), D5; slice 03 of the [approved map](../specs/harness-security-tickets.md).

**Scope:** The common dispatch decision plus every existing caller and transport path. Keep authentication credentials inside their trusted protocol adapter, not in prompts. If a provider cannot expose or safely reset retained context, stop rather than invent semantics. No live paid tests are authorized by this ticket.

**Blocked (2026-09-09):** Waiting for complete host provenance and lifecycle propagation.

## Acceptance Criteria

- Main, child, reader, guard, compactor and summarizer dispatches share effective-payload recipient enforcement.
- System/history/tools/attachments and provider-side state are covered rather than only the last message.
- Every retry revalidates authority; changed endpoints, redirects and fallbacks require an applicable recipient decision before transmission.
- Unknown retained remote context is reset or refused rather than treated as clean.
- Denied or unavailable dispatches explain the actual boundary in UI/headless output without secrets.

## Verification Plan

1. Exercise S07 and S14 using captured fake-provider requests across every role and transport attempt.
2. Test redirects, queue-time revocation, fallback, retained remote context, cancellation and missing metadata.
3. Before provider changes, verify authoritative service semantics and a reference implementation; run only scoped adapter/engine checks.
