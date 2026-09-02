---
id: lsp-hardening-release
title: Harden and release harness-managed LSP
status: done
priority: high
model_level: medium
task_type: feature
parent_id: harness-managed-lsp
tags:
    - lsp
    - security
    - reliability
acceptance_criteria:
    - Adversarial framing, URI, server-request, and output fixtures cannot escape baseline safety guarantees.
    - Cancellation, crash recovery, an exact circuit breaker, and shutdown leave no pending calls, processes, or goroutines.
    - Documentation and the system prompt route semantic questions to lsp and describe gopls-only scope.
    - CHANGELOG, race/fuzz suites, and make fmt-check lint test are complete.
verification_plan:
    - go test -race ./internal/lsp/... ./internal/tools/lsptool/... ./internal/agent/...
    - go test ./...
    - make fmt-check lint test
created_at: "2026-08-25T19:55:00Z"
updated_at: "2026-08-26T10:59:12.206012Z"
---

## Body

**Blocked by:** `lsp-shared-runtime-agents`, `lsp-navigation-operations`, `lsp-document-sync-diagnostics`, and `lsp-gopls-config-languages`.

This is the release gate, not the first implementation of basic safety. Ticket 2 already provides physical containment, server-request mutation denial, framing/file/result limits, bounded Close, and owner/query context separation. This ticket attacks those contracts with adversarial, race, leak, and fuzz coverage; completes recovery policy; and publishes accurate documentation.

### Security and resource review

Verify through `Manager.Query` that input and returned paths use the shared physical-containment seam, only local file URIs are accepted, and applyEdit/executeCommand cannot mutate files. Confirm trusted gopls argv/env comes only from secure global config and is redacted from every error/log/result. Document that trusted gopls has OS-level workspace access; the per-tool permission gate is not a syscall sandbox.

Audit the frozen limits from ticket 2 and ticket 5: 8 KiB headers, 8 MiB frame before allocation, 4 MiB file, 32 documents/32 MiB document cache, default 50/hard 200 items, 8 KiB text field, 64 KiB stderr tail, and 50 KiB final output. Any truncation reports omitted data; no full raw duplicate exists in metadata, transcript, or logs.

### Exact lifecycle policy

- Initialize and ordinary queries time out after 15 seconds; workspace symbols after 30 seconds; graceful shutdown after 2 seconds.
- Request cancellation sends `$/cancelRequest`, removes only its pending entry, and discards late responses.
- EOF, malformed framing, process exit, or writer failure retires the generation and fails every pending request.
- A later idempotent query may trigger a lazy restart; an individual query is never transparently executed more than twice.
- Circuit breaker key is canonical Go root. Record an attempt immediately before calling the process Starter; failed spawn and failed initialization consume quota, while config validation and missing-binary lookup do not. Allow at most three recorded attempts in a rolling 60-second window. A fourth returns typed unavailable with `retry_after_seconds = ceil((oldest_attempt + 60s - now) / 1s)`, minimum 1. Starts become eligible when the oldest timestamp leaves the window; a continuously healthy process naturally clears the window.
- Close is race-safe and idempotent: reject new → cancel pending → didClose documents → shutdown → exit → close stdin/wait → process-group kill after grace.

Fuzz Content-Length headers, JSON frames, URIs, Location/LocationLink shapes, markup, and diagnostic payloads. Race tests cover concurrent Query/Cancel/Close. Leak tests repeat start/crash/restart/close.

### Product integration

Add `doc/lsp.md` and a project-layout entry covering secure global config, gopls installation, operations, 1-based model coordinates, status/errors, limits, privacy/security, enabled=false, and troubleshooting. Update system prompt routing: exact text → grep; definition/type/references/callers/diagnostics → lsp. Do not advertise LSP when config explicitly disables the tool. Add a user-visible Unreleased CHANGELOG line.

Review public seams only: `Manager.Query`, `Manager.Close`, `lsptool.Tool(QueryFunc)`, and Engine role/registration behavior. Run separate Standards and Spec code reviews after the suite is green.

## Acceptance Criteria

- An adversarial server cannot make the harness expose an outside-workspace path or apply an edit.
- Oversized/malformed input fails before large allocation and all affected pending calls terminate.
- Query/Cancel/Close races and repeated crashes leave no process or goroutine.
- Circuit-breaker timing and retry_after are deterministic under a fake clock.
- Every model-visible result is bounded before transcript storage and contains no secret/raw protocol data.
- Prompt, docs, config, and CHANGELOG match gopls-only behavior.
- Standards and Spec reviews have no unresolved critical/high findings.

## Verification Plan

1. `go test -race ./internal/lsp/... ./internal/tools/lsptool/... ./internal/agent/...`
2. `go test ./...`
3. `make fmt-check lint test`
4. Run framing/URI/result fuzz corpora.
5. Smoke-test TUI, headless, concurrent children, crash/restart, missing gopls, and enabled=false.
6. Run `code-review` against the epic/spec before merge.
