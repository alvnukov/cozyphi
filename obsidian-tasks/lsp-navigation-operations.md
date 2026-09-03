---
id: lsp-navigation-operations
title: Add references, hover, symbols, and call hierarchy
status: done
priority: high
model_level: medium
task_type: feature
parent_id: harness-managed-lsp
tags:
    - lsp
    - gopls
    - navigation
acceptance_criteria:
    - The frozen lsp tool supports definition, references, hover, symbols, and calls without raw method forwarding.
    - Symbol targets resolve uniquely or return bounded candidates; protocol positions remain hidden.
    - Results are normalized, deduplicated, stable-sorted, and bounded before transcript storage.
    - Unsupported capabilities are distinguishable from successful empty results.
verification_plan:
    - go test ./internal/lsp/... ./internal/tools/lsptool/...
    - go test ./...
created_at: "2026-08-25T19:55:00Z"
updated_at: "2026-08-25T22:55:20.442719Z"
---

## Body

**Blocked by:** `lsp-gopls-definition-tracer`. May run in parallel with tickets 3, 5, and 6.

Implement the navigation handlers behind the schema, result variants, errors, limits, and operation-handler seam frozen by ticket 2. Do not change central schema or expose arbitrary protocol methods.

### Operations

- `definition`: add exact-symbol targeting. Resolve recursively flattened document symbols; zero matches is a bounded not-found result, multiple matches return the standard ambiguity candidates. Preserve ticket-2 exact-position behavior.
- `references`: call `textDocument/references`; use frozen `include_declaration`, default true, and deduplicate locations.
- `hover`: normalize plaintext, MarkedString, MarkupContent, and arrays into one bounded text field with an optional range.
- `symbols`: with `file`, flatten DocumentSymbol or normalize SymbolInformation; with `query`, call workspace/symbol. The frozen validator rejects both/neither.
- `calls`: internally perform prepareCallHierarchy followed by incoming/outgoing. Select the smallest containing item; equal candidates return the standard ambiguity error. Preserve opaque hierarchy `data` internally when making the second request and never expose it.

### Normalization

Locations use workspace-relative slash-separated paths and 1-based end-exclusive ranges. Symbols include bounded name, stable kind, detail, container, and location. Calls contain normalized from/to symbols and call sites. Unknown kinds use `unknown:<number>`.

The Manager enforces frozen default/hard item limits, per-field cap, and total result cap before returning structured Result. Sorting is path, start line/column, then name. Deduplicate by normalized identity. Do not retain a full raw duplicate in tool metadata.

Initialize capabilities are stored per client generation. Unsupported operations return typed unsupported errors; a supported null/empty response is a successful empty result. Server errors remain actionable rather than becoming empty output.

### TDD slices at public seams

Drive literal protocol fixtures through `Manager.Query`, not private normalizers:

1. Symbol definition resolves one target and returns candidates for ambiguity.
2. References honor include/exclude declaration and deduplicate.
3. Hover handles every allowed wire shape and truncates oversized content.
4. Nested document symbols flatten deterministically; workspace symbols honor query and limit.
5. Calls preserve opaque data and implement incoming/outgoing with deterministic ambiguity.
6. `lsptool.Tool(QueryFunc)` enforces operation-specific fields from the frozen schema.

## Acceptance Criteria

- All approved navigation operations work through the same `lsp` tool.
- Exact-symbol ambiguity never silently selects the first occurrence.
- Position conversion uses the same synchronized snapshot as the request.
- Raw URI, JSON, PID, argv, env, and stderr never reach Result or transcript metadata.
- Empty, unsupported, unavailable, ambiguous, and protocol failures are distinct.
- Truncation reports omitted counts instead of overflowing context.

## Verification Plan

1. `go test ./internal/lsp/... ./internal/tools/lsptool/...`
2. `go test -race ./internal/lsp/...`
3. `go test ./...`
4. Smoke-test references, hover, document/workspace symbols, and incoming/outgoing calls in CozyPhi.
