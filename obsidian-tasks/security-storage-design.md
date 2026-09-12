---
id: security-storage-design
title: 07 — Design encrypted session storage and key recovery boundaries
status: todo
priority: high
model_level: high
task_type: design
parent_id: harness-security-hardening
tags: [security, encryption, design]
acceptance_criteria:
  - An evidence-backed design inventories every sensitive persistence and export path, including child and temporary artifacts.
  - The design selects standard authenticated encryption and specifies format versioning, key/nonce lifecycle and passphrase parameters with rationale.
  - OS secure store or a separate passphrase protects keys with no adjacent plaintext key or plaintext fallback.
  - Migration, crash recovery, backups, tampering and unavailable/lost keys have explicit user-visible behavior.
  - Already encrypted sessions remain encrypted and resumable without guards when security is off; acceptance by the user or an authorized human reviewer is recorded before implementation.
verification_plan:
  - Trace the storage lifecycle and review S12, S13 and S15 against an artifact and failure-mode matrix.
  - Review candidate libraries and platform key-store constraints from authoritative documentation, including UI/headless unlock, unavailable-key and off-continuation scenarios.
  - Validate document links, decision completeness and public test scenarios; do not implement cryptography or run Go gates for document-only work.
created_at: "2026-09-09T11:16:39Z"
updated_at: "2026-09-09T11:16:39Z"
---

## Body

**What to build:** An implementable storage contract that lets a user understand setup, restart, off-mode continuation and key-loss consequences before sensitive sessions are enabled. Deliver the architecture and a reviewed user journey, not production encryption.

**Blocked by:** None — can start immediately.

**Contract:** [Specification](../specs/harness-security.md), D7; slice 07 of the [approved map](../specs/harness-security-tickets.md).

**Scope:** Inventory history, summaries, job transcripts, debug/export paths, temp files and backups. Choose concrete cryptographic dependencies and parameters only with evidence. Explain limits around old plaintext copies, OS compromise and server logs. Acceptance of this design is a prerequisite for 08; implementation must not silently fill unresolved security decisions.

## Acceptance Criteria

- An evidence-backed design inventories every sensitive persistence and export path, including child and temporary artifacts.
- The design selects standard authenticated encryption and specifies format versioning, key/nonce lifecycle and passphrase parameters with rationale.
- OS secure store or a separate passphrase protects keys with no adjacent plaintext key or plaintext fallback.
- Migration, crash recovery, backups, tampering and unavailable/lost keys have explicit user-visible behavior.
- Already encrypted sessions remain encrypted and resumable without guards when security is off; acceptance by the user or an authorized human reviewer is recorded before implementation.

## Verification Plan

1. Trace the storage lifecycle and review S12, S13 and S15 against an artifact and failure-mode matrix.
2. Review candidate libraries and platform key-store constraints from authoritative documentation, including UI/headless unlock, unavailable-key and off-continuation scenarios.
3. Validate document links, decision completeness and public test scenarios; do not implement cryptography or run Go gates for document-only work.
