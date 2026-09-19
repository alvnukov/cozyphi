---
id: web-research-33-protected-cache
title: 33 — Persist bounded web artifacts through shared OS-key-protected storage
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Sensitive web snapshots, normalized content, questions, answers, detailed incidents and checkpoints use shared authenticated protected storage with OS-backed keys.
    - TTL/size limits and missing/lost/tampered keys have explicit outcomes; no plaintext/passphrase fallback is introduced.
    - Ordinary model tools cannot access cache artifacts, including temp/log/crash routes; safe block metadata outlives eviction.
verification_plan:
    - Save/reopen owned sources with controlled key-store success/unavailable/lost/tampered cases and TTL/size eviction.
    - Inspect all runtime artifacts for synthetic plaintext markers and attempt reads through ordinary model tools/process boundaries.
    - Run only affected storage/web integration tests; record platform evidence and explicit limited-mode behavior.
created_at: "2026-09-19T19:18:45.211326Z"
updated_at: "2026-09-19T19:18:45.211326Z"
---

## Body

**What to build:** Bounded protected persistence for already implemented web research artifacts.

**Blocked by:** [13](web-research-13-source-followup.md), [storage design](security-storage-design.md), [encrypted sessions](security-encrypted-sessions.md).

**Unresolved prerequisite:** Proven process/filesystem access protection for the web artifact store, identified by 01 and the shared isolation owner. Encryption at rest alone is not that proof.

**Contract:** [Spec](../specs/protected-web-research.md), D7/D9/D11/D12/D16, Q20/Q22/Q35/Q40, T13/T24/T27. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Register web artifacts with the shared storage/key lifecycle; no custom crypto. Apply bounded retention and safe expired-reference outcomes. Integrate protected checkpoint recovery with 22 behavior when available, never auto-resume network. Audit temp files, exports, errors, logs and crash paths for plaintext. The selected web contract requires OS-backed key handling even if general security supports a separate passphrase. Safe temporary operation or refusal is the fallback.

**Do not change:** No adjacent key file, private decrypt tool accessible to model, promised secure deletion/recovery or cross-project shared content.

**Proof required:** Synthetic sensitive marker audit of all produced artifacts, wrong/lost key and tamper tests, retention eviction and actual denied ordinary-tool reads. Record OS key-store availability/limitations without real secrets.

**Stop condition:** Missing common storage/isolation primitives block this slice.

## Acceptance Criteria

- Sensitive web snapshots, normalized content, questions, answers, detailed incidents and checkpoints use shared authenticated protected storage with OS-backed keys.
- TTL/size limits and missing/lost/tampered keys have explicit outcomes; no plaintext/passphrase fallback is introduced.
- Ordinary model tools cannot access cache artifacts, including temp/log/crash routes; safe block metadata outlives eviction.

## Verification Plan

1. Save/reopen owned sources with controlled key-store success/unavailable/lost/tampered cases and TTL/size eviction.
2. Inspect all runtime artifacts for synthetic plaintext markers and attempt reads through ordinary model tools/process boundaries.
3. Run only affected storage/web integration tests; record platform evidence and explicit limited-mode behavior.
