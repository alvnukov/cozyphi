---
id: refactor-external-binary-runner
title: Establish one managed subprocess seam for commands and protocols
status: todo
priority: high
model_level: medium
task_type: refactor
parent_id: cozyphi-enterprise-code-review
tags:
    - process
    - reliability
    - lsp
acceptance_criteria:
    - One injected module starts argv without a shell and owns process-tree termination and Wait.
    - Run provides bounded command output; Start provides separate stdin/stdout and a bounded stderr tail.
    - Manager-lifetime cancellation or Close reaps the process without leaking goroutines.
    - bashtool and MCP stdio establish real command-mode and protocol-mode adapters.
    - The public interface contains no MCP or LSP concepts and has hermetic process tests.
verification_plan:
    - go test -race ./internal/tools/bashtool/... ./internal/mcp/...
    - go test ./...
created_at: "2026-08-23T15:17:22.118943Z"
updated_at: "2026-08-25T20:10:00Z"
---

## Body

This is the process-ownership prerequisite for `harness-managed-lsp`. Establish the shared seam without blocking LSP on a repository-wide migration.

**Blocked by:** None — can start immediately.

Create one deep subprocess module with a concrete, documented contract equivalent to:

```go
type Spec struct {
    Argv []string
    Dir  string
    Env  []string
}

func Run(ctx context.Context, spec Spec, limit Limit) (Result, error)
func Start(lifetime context.Context, spec Spec, stderrLimit int) (*Process, error)

// Process exposes protocol stdin/stdout, bounded StderrTail, Wait, and Close.
```

`Run` is for finite commands with bounded combined output. `Start` is for long-lived framed protocols: stdout must never be mixed with stderr, and the supplied context is the owner lifetime, not an individual protocol request. `Process.Close` is idempotent, terminates the process group, waits/reaps, and has a caller-provided grace deadline. Document supported Unix/Windows process-tree behavior and fail safely where equivalent group semantics are unavailable.

Migrate one existing command adapter (`bashtool`) and one protocol adapter (MCP stdio) to prove that the seam is real. Remaining grep/find/hooks migrations are follow-up cleanup and do not block LSP. Leaf packages receive the module through constructors/functions; project discovery remains in assembly.

TDD seam: exercise `Run`, `Start`, `Wait`, and `Close` through scripted helper processes. Do not assert private mutexes or goroutine structure. Errors include operation context but redact argv/env values. Content-Length framing and JSON-RPC routing are explicitly outside this module.

## Acceptance Criteria

- Empty argv and invalid working directories fail before process start.
- `Run` bounds stdout/stderr and honors cancellation.
- `Start` returns separate protocol streams and a bounded stderr tail.
- Owner-lifetime cancellation and repeated Close terminate and reap the whole supported process tree.
- A per-query context is not part of `Start`; cancelling one future LSP request cannot kill a shared server.
- bashtool and MCP stdio use the new seam; tests cover success, spawn failure, overflow, ignored termination, cancellation, and repeated Close.

## Verification Plan

1. `go test -race ./internal/tools/bashtool/... ./internal/mcp/...`
2. `go test ./...`
3. `make fmt-check lint test`