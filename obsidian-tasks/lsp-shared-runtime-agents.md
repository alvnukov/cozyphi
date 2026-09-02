---
id: lsp-shared-runtime-agents
title: Share one concurrent gopls runtime with child agents
status: done
priority: high
model_level: medium
task_type: feature
parent_id: harness-managed-lsp
tags:
    - lsp
    - agents
    - lifecycle
acceptance_criteria:
    - Primary, explore, review, and worker Engines use the same Manager generation without Start or Close authority.
    - Concurrent child queries route out-of-order responses by ID and never duplicate gopls for one root.
    - Engine rebind, mode/model changes, resume, and clear preserve the lsp tool without widening readonly roles.
    - Jobs close before the Manager; cancelling one request does not cancel initialization or other requests.
verification_plan:
    - go test -race ./internal/lsp/... ./internal/agent/... ./internal/job/... ./internal/tui/controller/... ./cmd/...
created_at: "2026-08-25T19:55:00Z"
updated_at: "2026-08-25T22:34:55.233855Z"
---

## Body

**Blocked by:** `lsp-gopls-definition-tracer`.

Share the working manager with every agent role without sharing lifecycle authority. Assembly owns `*lsp.Manager`; Engine and child factory receive only `QueryFunc`. Each `lsptool` adapter resolves relative file input against that Engine's own session cwd, then passes a canonical absolute path to the shared Manager, which repeats physical workspace validation.

### Required behavior

- TUI and headless assembly create one Manager per workspace process.
- Constructor parameters remain explicit; no dependency bag and no leaf `GetDefaultProject` calls.
- Job manager/EngineRunner passes QueryFunc to each child Engine.
- `buildToolList` adds bound `lsp` to default and readonly child tool sets without adding MCP or `agent_*` tools to children.
- SetModel, SetMode, plan rebind, Resume, and Clear preserve the capability.
- Start singleflight is keyed only by canonical Go root. The coalesced initialization task uses Manager lifetime context and survives cancellation of the first caller.
- One reader routes multiple in-flight responses by ID; writes remain serialized without a global I/O lock.
- Per-query cancellation sends `$/cancelRequest`, removes only that pending slot, and discards a late response. It does not retire a healthy process.
- Process/transport failure atomically fails all pending calls for that generation and removes the live root entry.
- Shutdown order is active loops → jobs/children → LSP Manager → remaining resources. Partial constructors close resources already created.
- Plan-step injection applies only to primary Engines with an active PlanGate. Children still pass hooks and permission checks but have no child plan lifecycle.

### TDD slices at public seams

1. Two `Manager.Query` calls receive reversed fake responses under the correct IDs.
2. Parent plus four children produce one observable initialize/process generation and shared fake call history.
3. Cancelling the first caller during initialize still lets the second caller succeed without a duplicate process.
4. Cancelling one child query does not affect a successful parent query.
5. Engine role and rebind matrices expose the expected tool set.
6. Assembly fakes observe jobs closing before Manager.Close.

## Acceptance Criteria

- Parent and child queries share one observable Manager/client generation per canonical root.
- Different child workdirs resolve relative file inputs correctly before manager validation.
- Out-of-order, late, and cancelled responses cannot desynchronize the connection.
- Readonly roles receive `lsp` as an exploration capability without a child PlanGate requirement.
- Engine and job packages cannot start, configure, or close gopls.
- Race tests and repeated Cancel/Close complete without leaks or hangs.

## Verification Plan

1. `go test -race ./internal/lsp/... ./internal/agent/... ./internal/job/...`
2. `go test ./internal/tui/controller/... ./cmd/...`
3. `go test ./...`
4. Run parent and two child definition queries concurrently and observe one gopls process.
