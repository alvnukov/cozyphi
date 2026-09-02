---
id: lsp-gopls-definition-tracer
title: 'LSP tracer: exact-position gopls definition end to end'
status: done
priority: high
model_level: medium
task_type: feature
parent_id: harness-managed-lsp
tags:
    - lsp
    - gopls
    - tools
acceptance_criteria:
    - The first definition lazily starts one gopls process and performs initialize, initialized, and didOpen.
    - The tool accepts an exact 1-based position and returns bounded workspace-relative Location or LocationLink results.
    - Ticket 2 freezes the complete V1 tool schema, result variants, typed errors, limits, and handler seam.
    - TUI and headless Engines register the tool through hooks, primary PlanGate, and read-like permission checks.
    - A fake framed server proves handshake, request ordering, ID routing, physical URI containment, and graceful Close.
verification_plan:
    - go test -race ./internal/lsp/... ./internal/tools/lsptool/... ./internal/agent/... ./cmd/...
    - go test ./...
created_at: "2026-08-25T19:55:00Z"
updated_at: "2026-08-25T21:47:01.08368Z"
---

## Body

**Blocked by:** `refactor-external-binary-runner` and `permission-symlink-workspace-escape`.

Deliver one narrow end-to-end tracer: `lsp` with `op=definition`, `file`, `line`, and `character` resolves the input against the Engine session cwd, passes physical containment, lazily starts gopls, performs the LSP handshake and didOpen, and returns a normalized definition in TUI and `cozyphi run`.

### Freeze the V1 contract

The single schema has `op`, `file`, `symbol`, `line`, `character`, `query`, `direction`, `include_declaration`, and `limit`. Runtime validation uses this fixed matrix:

- `languages`: no target fields;
- `definition`, `references`, `hover`, `calls`: `file` plus exactly one target (`symbol` or `line+character`); calls also requires `direction=incoming|outgoing`;
- `symbols`: exactly one of `file` or `query`;
- `diagnostics`: `file` only.

`include_declaration` applies only to references and defaults true. Limit defaults to 50 and has a hard maximum of 200. Unknown fields and irrelevant combinations fail before process start. Only exact-position definition is implemented in this ticket; symbol targeting and other operations return typed `unsupported` until ticket 4.

Freeze normalized result variants for locations, hover, symbols, calls, diagnostics, and languages plus `truncated`, `omitted`, and bounded warnings. Diagnostic freshness values are `fresh`, `cached`, `unconfirmed`, and `pending`. Freeze typed error categories: invalid, ambiguous, unsupported, unavailable, protocol, and closed; wrapped context cancellation remains discoverable. A private operation-handler table allows tickets 4–6 to add handlers without editing schema/result definitions or central validation.

Freeze assembly enablement as `Config.Enabled` defaulting true. `Open` returns `(nil, nil)` when disabled; assembly registers the tool only for a non-nil Manager. Ticket 3 exclusively owns consumption of this contract; ticket 6 only implements secure config loading behind it. Primary Engines receive injected `plan_step`; child Engines do not gain plans here. The tool adapter resolves relative input with its Engine cwd before calling Manager. Manager independently validates the resulting absolute path against its canonical workspace.

### Minimal manager behavior

- `Open` validates the workspace and starts no process.
- Go root selection is nearest `go.work`, then nearest `go.mod`, then workspace root, never above the workspace.
- One live client is keyed by canonical root. Start coalescing uses the Manager lifetime context, not the first Query context.
- JSON-RPC uses Content-Length framing, one reader loop, serialized writes, atomic IDs, and a pending map registered before write.
- Initialize sends the harness PID, root URI/folder, and only implemented capabilities, then sends `initialized`.
- didOpen uses a bounded disk snapshot and Go language ID before definition.
- Server requests are handled without the model: workspace folders, per-item configuration responses, applyEdit=false, and method-not-found for unsupported requests.
- Definition normalizes `Location`, `Location[]`, `LocationLink[]`, and null. A LocationLink uses `targetUri` and `targetSelectionRange` for the normalized navigation target; `targetRange` is not exposed in V1. Every returned file URI is decoded, canonicalized, and physically contained before entering Result.
- Close rejects new calls, sends shutdown/exit, then performs bounded kill/reap and joins transport goroutines.

Baseline limits are part of the tracer, not deferred hardening: 8 KiB headers, 8 MiB frame body before allocation, 4 MiB file, 8 KiB text field, 64 KiB stderr tail, default 50/hard 200 items, and 50 KiB final output. Malformed or oversized input retires the client generation.

### Tool and assembly

Add `internal/tools/lsptool`, a tools aggregation constructor, `ActionLSP` mapped to read policy, and exploration level 1 in PlanGate. `buildToolList` appends the bound tool before plan-step injection and preserves it across rebind. TUI/headless assembly owns one Manager and closes it after jobs. Child sharing remains ticket 3.

### TDD slices at public seams

1. Drive `Manager.Query` against a fake process: initialize → initialized → didOpen → definition → shutdown/exit.
2. Prove known Location and LocationLink fixtures become workspace-relative 1-based results.
3. Prove invalid positions, oversized frames, and escaping returned URIs fail before unsafe output.
4. Exercise schema/validation/rendering through `lsptool.Tool(QueryFunc)`.
5. Observe registration/rebind through Engine public behavior and gate extraction.
6. Exercise TUI/headless construction and partial-startup cleanup.

## Acceptance Criteria

- `Open` starts nothing; the first valid exact-position definition starts one process.
- A notification cannot consume a response, and response IDs are checked.
- Output uses bounded workspace-relative paths; raw URI/protocol payload is absent.
- Query cancellation does not cancel the Manager lifetime; Close leaves no process or goroutine.
- Missing gopls returns an install/configuration hint without breaking startup.
- Physical containment applies to both tool input and every returned file URI.
- The complete schema/result/error contract is frozen for parallel tickets 3–6.

## Verification Plan

1. `go test -race ./internal/lsp/... ./internal/tools/lsptool/...`
2. `go test ./internal/agent/... ./internal/permission/... ./internal/plangate/... ./cmd/...`
3. `go test ./...`
4. Manually query a definition in CozyPhi from TUI and `cozyphi run`.
