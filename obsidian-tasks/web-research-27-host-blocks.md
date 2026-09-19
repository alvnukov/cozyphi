---
id: web-research-27-host-blocks
title: 27 — Enforce exact-host incident blocks across user sessions and processes
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - One attributed quarantine call blocks the exact canonical hostname across user sessions/processes, schemes, ports, paths, redirects and cache references.
    - Canonicalization is consistent for case, trailing dot, IDN and IP forms; unrelated subdomains/tenants are not expanded into the block.
    - Automatic blocks, configured denials and network allowances remain separate; cache eviction cannot clear a block.
verification_plan:
    - Run two-process shared-state integration with case/IDN/trailing-dot/IP and scheme/port/path variants.
    - Observe forbidden acquisition/release sinks after block and permitted unrelated-host control.
    - Evict snapshots and restart clients; attach block-version and sink evidence plus scoped race tests.
created_at: "2026-09-19T19:17:19.005636Z"
updated_at: "2026-09-19T19:17:19.005636Z"
---

## Body

**What to build:** Apply a source-attributed incident as a user-wide exact-host admission block through a shared coordination owner.

**Blocked by:** [26](web-research-26-incident-kinds.md).

**Unresolved prerequisite:** Before reopening, use the inventory from 01 to identify and record the concrete approved/landed shared host-coordination implementation. No such implementation is assumed by this ticket; do not create an ad-hoc global service in a medium integration slice.

**Contract:** [Spec](../specs/protected-web-research.md), D9/D11, Q6/Q20/Q27/Q28, T17/T18/T19. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Reuse canonical destination identity from acquisition and shared atomic state. Check block state before fetch, cache/source use and release. Persist only safe necessary global incident/block metadata via the agreed mechanism. Keep user-configured denylist and automatic evidence independent; explicit allowlists do not override either protection. State unavailable if coordination cannot guarantee visibility.

**Do not change:** No registrable-domain widening, last-write-wins unblock, TTL-based unblock or claim that a single-process map is global.

**Proof required:** Two real process clients: a block in one prevents all hostname variants in the other while a sibling subdomain control remains unaffected. Verify block persists after snapshot eviction/restart.

**Stop condition:** Missing coordination, normalization or safe persistence contract remains a hard blocker.

## Acceptance Criteria

- One attributed quarantine call blocks the exact canonical hostname across user sessions/processes, schemes, ports, paths, redirects and cache references.
- Canonicalization is consistent for case, trailing dot, IDN and IP forms; unrelated subdomains/tenants are not expanded into the block.
- Automatic blocks, configured denials and network allowances remain separate; cache eviction cannot clear a block.

## Verification Plan

1. Run two-process shared-state integration with case/IDN/trailing-dot/IP and scheme/port/path variants.
2. Observe forbidden acquisition/release sinks after block and permitted unrelated-host control.
3. Evict snapshots and restart clients; attach block-version and sink evidence plus scoped race tests.
