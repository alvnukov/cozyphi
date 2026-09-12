---
id: security-trusted-profiles
title: 09 — Use trusted profiles across roles and confirm sensitive transfers
status: blocked
priority: high
model_level: high
task_type: feature
parent_id: harness-security-hardening
tags: [security, models, approval]
acceptance_criteria:
  - Only explicitly warned local/own provider-endpoint-model profiles receive raw sensitive context, with visible identity-bound trust.
  - Main, child and summarizer raw-secret access requires encrypted storage readiness; trust never grants tool authority.
  - New profiles and changed endpoints/models do not inherit trust; fallback and revocation use the same recipient boundary.
  - Enabled sensitive-to-untrusted transitions require clean context, warned redacted transfer or cancel; known remaining secrets are not sent.
  - Redaction and summaries do not automatically declassify data; off disables guards/dialogs while preserving encryption and a visible transmission warning.
verification_plan:
  - Exercise S11 and S14 across main, child, summarizer and profile fallback with captured provider payloads.
  - Test warning acceptance, identity changes, unavailable keys, raw-secret rollout gating and retained remote state.
  - Run scoped model-selection/session/UI tests; verify provider semantics before adapter changes and use no live paid calls.
created_at: "2026-09-09T11:16:39Z"
updated_at: "2026-09-09T11:16:39Z"
---

## Body

**What to build:** A user can knowingly use an eligible trusted profile for every role of an encrypted sensitive session and make an explicit choice before moving that context to an untrusted recipient.

**Blocked by:** security-model-egress, security-encrypted-sessions.

**Contract:** [Specification](../specs/harness-security.md), D7–D8; slice 09 of the [approved map](../specs/harness-security-tickets.md).

**Scope:** Profile acceptance and revocation, role-aware dispatch, storage readiness, clean/redacted/cancel UI and headless behavior. Reuse the guard trust contract when available, with no dependency on guard inference. Required warnings cover secret access, server logs/forwarding, injection/model errors and inability to certify isolation. The redaction warning explicitly states that removing known secrets may leave sensitive or derived data.

**Blocked (2026-09-09):** Waiting for every-dispatch enforcement and protected storage.

## Acceptance Criteria

- Only explicitly warned local/own provider-endpoint-model profiles receive raw sensitive context, with visible identity-bound trust.
- Main, child and summarizer raw-secret access requires encrypted storage readiness; trust never grants tool authority.
- New profiles and changed endpoints/models do not inherit trust; fallback and revocation use the same recipient boundary.
- Enabled sensitive-to-untrusted transitions require clean context, warned redacted transfer or cancel; known remaining secrets are not sent.
- Redaction and summaries do not automatically declassify data; off disables guards/dialogs while preserving encryption and a visible transmission warning.

## Verification Plan

1. Exercise S11 and S14 across main, child, summarizer and profile fallback with captured provider payloads.
2. Test warning acceptance, identity changes, unavailable keys, raw-secret rollout gating and retained remote state.
3. Run scoped model-selection/session/UI tests; verify provider semantics before adapter changes and use no live paid calls.
