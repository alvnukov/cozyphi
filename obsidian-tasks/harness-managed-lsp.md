---
id: harness-managed-lsp
title: Harness-managed LSP behind one model-facing tool
status: todo
priority: high
model_level: medium
task_type: feature
tags:
    - lsp
    - agent
    - tools
    - code-intelligence
acceptance_criteria:
    - The model uses one read-only lsp tool and cannot manage processes or send raw LSP methods.
    - The harness selects, starts, initializes, synchronizes, recovers, and closes gopls.
    - Primary and child agents share one workspace manager with independent request cancellation.
    - Definition, references, hover, symbols, calls, diagnostics, and languages return bounded normalized results.
    - Only owner-controlled ~/.cozyphi/lsp.json configures gopls; project-local executable config is unsupported.
verification_plan:
    - go test -race ./internal/lsp/... ./internal/tools/lsptool/... ./internal/agent/...
    - go test ./...
    - make fmt-check lint test
created_at: "2026-08-25T19:55:00Z"
updated_at: "2026-08-25T20:10:00Z"
---

## Body

Add a deep `internal/lsp` module that hides configuration, gopls selection, root discovery, subprocesses, Content-Length JSON-RPC, initialization, document versions, position encoding, diagnostics, cancellation, restart, and shutdown. The model gets one compact `lsp` tool. Server names, argv/env, arbitrary methods, restart, and shutdown are never tool arguments.

### Frozen external seam

Assembly owns one concrete process-scoped `Manager`. Engines receive only a borrowed `QueryFunc`, so they cannot close the shared runtime. `Query` returns a normalized structured `Result`; `internal/tools/lsptool` renders it within transcript limits.

```go
type QueryFunc func(context.Context, Query) (Result, error)

func Open(lifetime context.Context, workspace string, config Config) (*Manager, error)
func (m *Manager) Query(context.Context, Query) (Result, error)
func (m *Manager) Close(context.Context) error
```

The manager lifetime context owns subprocesses. Each Query context only controls one request and may send `$/cancelRequest`; it never kills the shared gopls process. Public coordinates are 1-based Unicode code-point positions. Wire positions use the negotiated encoding, defaulting to UTF-16. Output paths are always workspace-relative with slash separators; session cwd is used only by the tool adapter to resolve input.

Ticket 2 freezes the complete V1 input matrix, result variants, typed errors, limits, operation-handler seam, and assembly-facing enablement contract. `Config.Enabled` defaults true; `Open` returns `(nil, nil)` when false, and assembly registers `lsp` only for a non-nil Manager. Ticket 3 exclusively consumes that contract in TUI/headless/child wiring. Ticket 6 implements secure config loading behind it and does not edit assembly. Tickets 3–6 otherwise add disjoint behavior without changing the central schema. Unimplemented operations return typed `unsupported` errors until their ticket lands.

### Scope

V1 supports gopls only. There is no Rust/TypeScript/Python catalog, generic arbitrary-server adapter, automatic download, or project-local `.cozyphi/lsp.json`. A second production server is required before introducing a server-adapter seam.

### OpenCode reference map

Use the implementation under `~/src/opencode/packages/opencode` as a reference:

- pool, start coalescing, and root routing: `src/lsp/lsp.ts` (`getClients`);
- framing, initialization, sync, requests, and diagnostics: `src/lsp/client.ts`;
- gopls root/spawn behavior: `src/lsp/server.ts` (`Gopls`);
- fake framed server: `test/fixture/lsp/fake-lsp-server.js`;
- tool shape: `src/tool/lsp.ts`.

Intentional divergences: gopls only, no auto-download, no permanent broken set, actionable errors rather than silent empty values, bounded documents/results, physical path containment, and graceful `shutdown`/`exit`.

### Ticket graph

1. `refactor-external-binary-runner` establishes the subprocess seam.
2. `lsp-gopls-definition-tracer` is blocked by ticket 1 and `permission-symlink-workspace-escape`; it freezes the full contract and delivers exact-position definition.
3. `lsp-shared-runtime-agents` is blocked by ticket 2.
4. `lsp-navigation-operations` is blocked by ticket 2.
5. `lsp-document-sync-diagnostics` is blocked by ticket 2.
6. `lsp-gopls-config-languages` is blocked by ticket 2.
7. `lsp-hardening-release` is blocked by tickets 3–6.

Tickets 3–6 intentionally run in parallel after ticket 2. File ownership is fixed: ticket 3 exclusively owns child/TUI/headless assembly and consumes frozen enablement; ticket 4 owns navigation handlers; ticket 5 owns document/diagnostic state; ticket 6 owns config loading and languages status behind the frozen assembly contract. They do not extend the central schema.

## Acceptance Criteria

- One `lsp` schema exposes all approved operations without lifecycle or raw-protocol controls.
- The harness owns the full gopls lifecycle and is shared by primary and child agents.
- Calls preserve the executor order: PreHooks → PlanGate when active → Gate/Ask → Run → PostHooks.
- Input and returned paths fail closed on physical workspace escape before reaching Result.
- Results are normalized, sorted, deduplicated, and bounded before transcript storage.
- Missing gopls does not break startup and produces an install hint; malformed explicit config fails closed.

## Verification Plan

1. Complete every child ticket verification plan.
2. `go test -race ./internal/lsp/... ./internal/tools/lsptool/... ./internal/agent/...`
3. `go test ./...`
4. `make fmt-check lint test`
5. Smoke-test all operations in TUI, headless mode, and concurrent child agents.