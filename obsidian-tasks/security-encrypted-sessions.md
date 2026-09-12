---
id: security-encrypted-sessions
title: 08 — Persist and resume encrypted sensitive sessions and derivatives
status: blocked
priority: high
model_level: high
task_type: feature
parent_id: harness-security-hardening
tags: [security, encryption, session]
acceptance_criteria:
  - The accepted storage design protects the entire sensitive session and every identified derived artifact, not just secret matches.
  - Keys use the accepted OS-store or separate-passphrase path; key errors and tampering fail without plaintext fallback.
  - Resume, compaction, child transcripts, crash recovery and backups preserve encryption and provenance.
  - Turning security off permits encrypted continuation without guards and never decrypts persistent history.
  - UI/headless behavior distinguishes encrypted storage from disabled transmission protection and explains key-loss limits.
verification_plan:
  - Exercise S12, S13 and S15 through public session operations and real storage adapters with synthetic canaries.
  - Inspect all inventoried artifacts and failure outputs; test locked/missing keys, tampering, interrupted writes, migration and restart.
  - Run only affected storage/lifecycle tests, race checks where warranted and one scoped lint before commit.
created_at: "2026-09-09T11:16:39Z"
updated_at: "2026-09-09T11:16:39Z"
---

## Body

**What to build:** A user creates, restarts, compacts and continues a sensitive encrypted session, including after switching runtime protection off, without leaving plaintext derivatives. Implement only the accepted storage contract and its recovery journey.

**Blocked by:** security-durable-provenance, security-storage-design.

**Contract:** [Specification](../specs/harness-security.md), D7; slice 08 of the [approved map](../specs/harness-security-tickets.md).

**Scope:** Protected session lifecycle, key access, derived artifact persistence, migration/compatibility and storage-format invariants. The off switch disables runtime checking, not existing encryption. New ordinary off sessions keep legacy behavior. Do not enable all trusted model roles until slice 09 verifies the full dispatch/switch path.

**Blocked (2026-09-09):** Waiting for provenance and explicit acceptance of the cryptographic storage design.

## Acceptance Criteria

- The accepted storage design protects the entire sensitive session and every identified derived artifact, not just secret matches.
- Keys use the accepted OS-store or separate-passphrase path; key errors and tampering fail without plaintext fallback.
- Resume, compaction, child transcripts, crash recovery and backups preserve encryption and provenance.
- Turning security off permits encrypted continuation without guards and never decrypts persistent history.
- UI/headless behavior distinguishes encrypted storage from disabled transmission protection and explains key-loss limits.

## Verification Plan

1. Exercise S12, S13 and S15 through public session operations and real storage adapters with synthetic canaries.
2. Inspect all inventoried artifacts and failure outputs; test locked/missing keys, tampering, interrupted writes, migration and restart.
3. Run only affected storage/lifecycle tests, race checks where warranted and one scoped lint before commit.
