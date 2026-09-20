---
id: web-research-04-consented-preflight
title: 04 — Check the selected web route only after bounded consent
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - No preflight request occurs before explicit consent describing recipient and bounded quota use.
    - Actual route checks vision, executor-less tool calls and structured results; unsupported/unknown behavior is not-ready.
    - Binding changes or runtime capability failures invalidate readiness; no OCR-only, main-model or provider substitution occurs.
verification_plan:
    - Drive consent accept/deny/unavailable through the public interaction boundary using deterministic provider fixtures and the shared admission API.
    - Verify images and decoy tool events have no real executor; malformed replies and unsupported account/route adapters cannot pass.
    - Record request counts, effective route, claim reconciliation, capability outcomes and scoped commands; no paid/live trial without separate approval.
created_at: "2026-09-19T19:10:37.855215Z"
updated_at: "2026-09-20T22:31:41.321796Z"
---

## Body

**What to build:** A consented capability check for the user's selected web route, with an honest ready/limited/unavailable outcome.

**Blocked by:** [03](web-research-03-model-binding.md), [model egress](security-model-egress.md), [shared account-admission foundation](routing-openai-account-admission.md).

**Contract:** [Spec](../specs/protected-web-research.md), D2/D8, Q9/Q16/Q38/Q41, T01/T04/T15. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Send a small bounded fixture set through the real provider abstraction under consent and delivered shared account admission. The named routing task owns the common admit/wait/claim/reconcile seam; its first OpenAI adapter does not prescribe the web provider. If the selected route lacks a compatible proven account adapter, remain unavailable until that owner's implementation lands. Use synthetic text, image and decoy-call responses; no real executor. Bind results to effective configuration and capability requirements, distinguish metadata from observed support, and surface unsupported full-mode capability. Runtime failure stays fail-closed after successful preflight. Attach authoritative evidence before changing provider protocol behavior. This capability check does not enable general research; aggregate readiness belongs to 41.

**Do not change:** No live requests during offline development, guessed vision flags, unbounded retries, certification claim, private scheduler or fallback model.

**Proof required:** Offline request captures before/after consent, unsupported tool/image/schema cases and binding changes; zero pre-consent calls, exact route and shared claim reconciliation throughout. A live preflight needs separately approved scope/budget.

**Stop condition:** Unknown capabilities, recipient identity or shared account support block readiness, not implementation of an optimistic default.

**Note (2026-09-21).** 2026-09-06: реализация завершена на ветке feature/web-research-04-consented-preflight (worktree). Proof: 4 новых agent-теста (web_preflight_admission_test.go) зелёные — undelivered-adapter отказ naming routing-openai-account-admission без запросов/асков; consent 1 раз с Action=web_preflight, кэш вердикта по fingerprint маршрута; deny кэшируется; смена BaseURL → повторный preflight. Регрессия: go test agent/permission/webpreflight ok, go build ./internal/... чисто, gofmt чисто, один scoped golangci-lint run (5 замечаний исправлено: echoMarker rename для G101, errors.New ×2, _ int, инлайн sseToolCall). Проводка: webRuntime кэширует Verdict по WebBinding.Fingerprint; webAdmission → webpreflight.New(model, identity, decoys, admit, consent через engine.gate).

## Acceptance Criteria

- No preflight request occurs before explicit consent describing recipient and bounded quota use.
- Actual route checks vision, executor-less tool calls and structured results; unsupported/unknown behavior is not-ready.
- Binding changes or runtime capability failures invalidate readiness; no OCR-only, main-model or provider substitution occurs.

## Verification Plan

1. Drive consent accept/deny/unavailable through the public interaction boundary using deterministic provider fixtures and the shared admission API.
2. Verify images and decoy tool events have no real executor; malformed replies and unsupported account/route adapters cannot pass.
3. Record request counts, effective route, claim reconciliation, capability outcomes and scoped commands; no paid/live trial without separate approval.
