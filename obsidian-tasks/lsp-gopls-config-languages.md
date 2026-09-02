---
id: lsp-gopls-config-languages
title: Add secure owner-controlled gopls config and languages status
status: done
priority: medium
model_level: medium
task_type: feature
parent_id: harness-managed-lsp
tags:
    - lsp
    - gopls
    - config
acceptance_criteria:
    - ~/.cozyphi/lsp.json configures gopls only and is loaded as a secure owner-controlled regular file.
    - Missing config uses the built-in gopls profile; malformed or insecure explicit config fails closed.
    - languages reports configured, installed, running, and bounded error fields without process start or download.
    - Config, env, argv, and settings secrets never enter logs, tool output, or transcript.
verification_plan:
    - go test ./internal/lsp/... ./internal/project/... ./cmd/...
    - go test ./...
created_at: "2026-08-25T19:55:00Z"
updated_at: "2026-08-26T07:13:28.953889Z"
---

## Body

**Blocked by:** `lsp-gopls-definition-tracer`. May run in parallel with tickets 3, 4, and 5.

Implement the frozen languages handler and V1 configuration for one production server: gopls. Do not add Rust/TypeScript/Python profiles or a generic arbitrary-server adapter before a second real adapter exists.

### Configuration contract

Add the global layout path `~/.cozyphi/lsp.json`. A missing file means built-in defaults. An existing malformed, semantically invalid, symlinked, non-regular, wrong-owner, or group/world-writable file fails closed with a sanitized error. Use no-follow/open-and-verify semantics where supported; document platform ownership/mode limitations. CozyPhi-created files use owner-only permissions.

```json
{
  "enabled": true,
  "gopls": {
    "command": ["gopls"],
    "env": {},
    "initialization_options": {},
    "settings": {}
  }
}
```

- command is non-empty argv launched without a shell and never model-controlled. Its first element must be either an absolute path or a bare basename resolved through owner bin/PATH; reject `./gopls`, `../gopls`, volume-relative paths, and any other non-absolute value containing a platform path separator.
- env adds to a sanitized inherited environment; values are never rendered.
- initialization_options appear only in initialize.
- settings serve workspace/configuration section lookup and didChangeConfiguration. Each workspace/configuration item gets one result: the requested dotted section or null.
- Unknown top-level keys, server keys, and fields fail closed. Only `gopls` is valid.
- `enabled:false` means no `lsp` tool is registered. There is no environment-variable disable override in V1.
- Missing gopls while enabled still registers `lsp`, so `languages` can return an install hint.
- Project-local `.cozyphi/lsp.json` is unsupported and never read.

Root markers remain built-in: go.work, then go.mod, then workspace fallback. Binary lookup accepts an absolute configured executable directly; otherwise it resolves a bare basename through `~/.cozyphi/bin` and then PATH. No working-directory-relative executable and no network, download, or install action is allowed.

### languages result

`op=languages` requires no target fields and never calls the process starter. It returns one bounded Go/gopls record with separate fields rather than overlapping states:

- `language: go` and `server: gopls`;
- `configured: true`;
- `installed: bool`;
- `running: bool` and active root count, without PID;
- bounded optional `error`;
- known or initialized supported operations;
- install hint `go install golang.org/x/tools/gopls@latest` when missing, never executed.

### TDD slices at public seams

1. Load config through the public loader/Manager construction: missing defaults; malformed, symlinked, insecure-mode, wrong-owner where supported, empty command, and unknown server fail closed.
2. Fake initialize/configuration proves settings and initialization options reach only their defined fields and secrets stay out of errors.
3. Hermetic resolver proves binary lookup precedence without real HOME/PATH.
4. `Manager.Query(languages)` never starts a process and reports installed/running/error booleans.
5. Project-local executable config is not read.
6. Config loading returns the frozen enabled/disabled value without editing TUI/headless assembly; ticket 3 owns registration behavior.

## Acceptance Criteria

- V1 supports only gopls and introduces no hypothetical multi-server adapter seam.
- Missing gopls gives an actionable status without startup failure or auto-install.
- Explicit malformed or insecure config never silently falls back to defaults.
- A project checkout cannot alter command, env, initialization options, or settings.
- languages exposes no PID, argv, env values, secret settings, or raw stderr.
- Config tests do not use the real HOME, ownership, or PATH state.
- Cwd-relative executable forms are rejected on Unix and Windows path fixtures.

## Verification Plan

1. `go test ./internal/lsp/... ./internal/project/... ./cmd/...`
2. `go test -race ./internal/lsp/...`
3. `go test ./...`
4. Smoke-test missing config, missing gopls, custom argv, enabled=false, malformed config, symlink, and insecure mode.
