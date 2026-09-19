---
id: web-research-06-single-source-tracer
title: 06 — Complete one offline source research through the public boundary
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Public question submission returns a job ID and processes exactly one controlled immutable source.
    - Safety and extraction are separate concurrent executor-less calls; a release barrier joins them and a third call screens the exact candidate.
    - Only a checked candidate with host-owned source identity can release in offline tests; production acquisition stays disabled until aggregate readiness.
verification_plan:
    - Submit through session/web with controlled source/provider/account adapters; verify prompt job identity and one checked result.
    - Prove both initial calls become eligible before completion and compare screened/emitted candidate; account capacity may serialize actual dispatch.
    - Force safety/final denial and cancellation; assert no drafts/tool execution, no leaked shared claims and production acquisition stays unavailable.
created_at: "2026-09-19T19:11:58.976971Z"
updated_at: "2026-09-19T19:32:43.830449Z"
---

## Body

**What to build:** The smallest end-to-end research tracer bullet using one injected offline snapshot, through the public web/session boundary rather than a private pipeline test.

**Blocked by:** [03](web-research-03-model-binding.md), [05](web-research-05-research-consent.md), [shared account-admission foundation](routing-openai-account-admission.md).

**Contract:** [Spec](../specs/protected-web-research.md), D1/D3/D5/D6/D12, Q15/Q17/Q24, T03/T04/T06. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Reuse job identity, cancellation and the delivered shared admit/claim/reconcile contract. The foundation's provider-specific title does not pin web to that provider; tests use controlled supported account/provider adapters. Keep one deep orchestration boundary with bounded outputs and host-owned source/candidate identities. Independent safety/extraction contexts receive permitted question/material and decoy definitions, never executors/history. Join passes before a third quarantined call screens the exact final candidate. Emit safe status and one untrusted answer through controlled test delivery. This establishes minimal happy-path and stage refusals; fixtures are never a user-accessible isolation bypass. Production research remains disabled until aggregate gate 41 and applicable runtime readiness checks; 19 later integrates background priority and pins, not the foundation used here.

**Do not change:** No search, network, persistence, multi-source synthesis, pipeline DSL, public interface per stage, private scheduler or model-selected orchestration.

**Proof required:** Public submission/status/result trace and shared admission captures demonstrate concurrent eligible calls, bounded contexts, claim cleanup and final screening of the emitted candidate.

**Stop condition:** Missing shared authority/admission/schema requirements block the slice; do not fill them with permissive defaults.

## Acceptance Criteria

- Public question submission returns a job ID and processes exactly one controlled immutable source.
- Safety and extraction are separate concurrent executor-less calls; a release barrier joins them and a third call screens the exact candidate.
- Only a checked candidate with host-owned source identity can release in offline tests; production acquisition stays disabled until aggregate readiness.

## Verification Plan

1. Submit through session/web with controlled source/provider/account adapters; verify prompt job identity and one checked result.
2. Prove both initial calls become eligible before completion and compare screened/emitted candidate; account capacity may serialize actual dispatch.
3. Force safety/final denial and cancellation; assert no drafts/tool execution, no leaked shared claims and production acquisition stays unavailable.
