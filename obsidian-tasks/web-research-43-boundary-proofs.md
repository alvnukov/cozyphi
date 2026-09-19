---
id: web-research-43-boundary-proofs
title: 43 — Prove actual web action, egress, filesystem and process boundaries
status: blocked
priority: high
model_level: medium
task_type: test
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Tests observe actual permitted/forbidden action, network, filesystem and process boundaries for enabled channels.
    - Raw/cache, private network, secret inheritance, parser/browser escape and cross-project access attempts reach no forbidden sink.
    - Coverage is per platform/channel; mock-only or unavailable runs cannot certify OS isolation.
verification_plan:
    - Run targeted platform integration cases against synthetic controlled sinks and fresh restricted processes.
    - Test each enabled channel and one positive control; inspect actual dispatch, files, descendants and outbound bytes.
    - Attach exact platform/tool versions, scoped commands, zero-bypass observations and explicit unavailable coverage.
created_at: "2026-09-19T19:21:48.042332Z"
updated_at: "2026-09-19T19:21:48.042332Z"
---

## Body

**What to build:** Focused integration proofs for the already implemented platform barriers, not a new security framework or sandbox.

**Blocked by:** [10](web-research-10-public-acquisition.md), [25](web-research-25-post-web-grants.md), [33](web-research-33-protected-cache.md), [34](web-research-34-source-transfer.md), [37](web-research-37-text-pdf.md), [38](web-research-38-rendered-pages.md), [39](web-research-39-visual-sources.md), [40](web-research-40-subresource-revocation.md), [41](web-research-41-restricted-mode.md).

**Contract:** [Spec](../specs/protected-web-research.md), Testing Decisions, T11–T13/T15–T17/T24/T25/T30/T31/T32. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Extend the existing platform integration fixtures with synthetic secrets and controlled sinks. Try direct execution as well as model-advertised paths, descendants, queued payload changes and access to cache/checkpoint/incident artifacts. Include grant-authorized and public-network positive controls. Reuse proof hooks from completed isolation prerequisites; each scoped run records actual OS/adapter coverage.

**Do not change:** No production isolation redesign, attacks against real user resources, plaintext real secrets, global gates or claim that deny-list unit tests prove containment.

**Proof required:** Case-to-platform matrix with actual sink counters/files/process outcomes and bounded cleanup results. Distinguish blocked capability, skipped test, deterministic pass and unproved platform.

**Stop condition:** A missing platform fixture or discovered bypass is a blocking gap to its implementation owner; never replace it with a permissive mock.

## Acceptance Criteria

- Tests observe actual permitted/forbidden action, network, filesystem and process boundaries for enabled channels.
- Raw/cache, private network, secret inheritance, parser/browser escape and cross-project access attempts reach no forbidden sink.
- Coverage is per platform/channel; mock-only or unavailable runs cannot certify OS isolation.

## Verification Plan

1. Run targeted platform integration cases against synthetic controlled sinks and fresh restricted processes.
2. Test each enabled channel and one positive control; inspect actual dispatch, files, descendants and outbound bytes.
3. Attach exact platform/tool versions, scoped commands, zero-bypass observations and explicit unavailable coverage.
