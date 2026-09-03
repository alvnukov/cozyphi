---
id: lsp-document-sync-diagnostics
title: Keep disk-backed documents and gopls diagnostics current
status: done
priority: high
model_level: medium
task_type: feature
parent_id: harness-managed-lsp
tags:
    - lsp
    - diagnostics
    - reliability
acceptance_criteria:
    - Before every document query the Manager synchronizes the current disk snapshot without model or write-tool commands.
    - didOpen, didChange, and didClose follow negotiated sync kind and monotonic versions.
    - Current push and pull diagnostics are merged without claiming unversioned data is fresh.
    - Results report fresh, cached, unconfirmed, or pending under explicit precedence rules.
    - Document and diagnostic caches are bounded and cleared on client restart.
verification_plan:
    - go test -race ./internal/lsp/... ./internal/tools/lsptool/...
    - go test ./...
created_at: "2026-08-25T19:55:00Z"
updated_at: "2026-08-26T06:43:13.89809Z"
---

## Body

**Blocked by:** `lsp-gopls-definition-tracer`. May run in parallel with tickets 3, 4, and 6.

Implement the frozen diagnostics handler and make document synchronization fully harness-managed. The model, read/write/edit/bash tools, and child agents never send lifecycle notifications. Before every document operation, Manager reads the disk snapshot, compares its content hash, and completes a write barrier before the semantic request.

### Document state

- First use sends didOpen with full text, Go language ID, and version 1.
- An unchanged content hash sends no notification and may reuse confirmed diagnostics.
- Changed text increments the version. Full-sync servers receive full text; incremental servers receive one replacement range computed from the old snapshot in negotiated encoding.
- Sync kind None sends no text notifications but does not invent support.
- Versions are never reused within a client generation.
- An LRU holds at most 32 open documents and 32 MiB total text; eviction sends didClose.
- File-size and UTF-8 validity checks happen before didOpen. Position conversion uses that exact synchronized snapshot.
- Restart clears open-document and diagnostic state; the next query reopens current disk content.

Read-through synchronization catches changes from CozyPhi tools, shell commands, child agents, formatters, and external editors without coupling each writer to LSP. A filesystem watcher and background diagnostics are outside V1.

### Diagnostics policy

Maintain push and pull caches separately, then merge and deduplicate only entries proven current for the synchronized snapshot:

- If static `diagnosticProvider` is advertised, request `textDocument/diagnostic` after the sync barrier and support full/unchanged reports with resultId.
- Always accept publishDiagnostics replacement notifications. A matching document version is current. An unversioned publication can never prove freshness: retain it as `unconfirmed`, whether it arrives before or after the sync barrier.
- Older or mismatched versioned publications are ignored. An empty matching-version publication clears the push cache; an empty unversioned publication clears only the unconfirmed push cache.
- Current pull and matching-version push entries are merged and deduplicated by severity/code/source/message/range. Pull does not silently erase unrelated current versioned push diagnostics.
- For an unchanged snapshot, the last confirmed merged result may return `cached`. Post-barrier unversioned data may return `unconfirmed` with a bounded warning, never `fresh`.
- If no confirmed or unconfirmed result arrives within 5 seconds, return `pending`, never a false empty-success claim.
- V1 does not advertise dynamic diagnostic registration. Register/unregister/refresh requests receive the ticket-2 unsupported handling.
- Related documents pass URI normalization and physical workspace containment before caching.

### TDD slices at public seams

Drive all cases through `Manager.Query` and fake protocol history:

1. First query sends didOpen; an identical second query sends no didChange.
2. A disk edit sends a newer didChange before the next request.
3. Incremental replacement is correct for Unicode/UTF-16.
4. LRU eviction sends didClose and enforces memory caps.
5. Matching-version push is fresh; stale versions are ignored; unversioned push is unconfirmed even after the barrier; empty notifications clear only their own cache class.
6. Pull full/unchanged plus current push merge/dedup and bounded unconfirmed/pending behavior.
7. Crash/restart never returns diagnostics from the old generation.

## Acceptance Criteria

- No tool argument controls open/change/close/version.
- The next query observes current disk content after internal or external edits.
- A semantic request never overtakes its didOpen/didChange barrier.
- Fresh, cached, unconfirmed, and pending semantics are pinned by literal protocol fixtures.
- The 32-document/32 MiB caps prevent unbounded text retention; frozen item/output caps bound diagnostics.
- Out-of-workspace and non-file related URIs are omitted with a bounded warning.

## Verification Plan

1. `go test -race ./internal/lsp/...`
2. `go test ./internal/tools/lsptool/...`
3. `go test ./...`
4. Introduce and fix a Go compile error via both edit and an external editor; diagnostics must update and clear.
