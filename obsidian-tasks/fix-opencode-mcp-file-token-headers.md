---
id: fix-opencode-mcp-file-token-headers
title: OpenCode MCP headers/environment do not expand {file:...} references
status: in_progress
priority: critical
task_type: bug
tags:
    - opencode
    - mcp
    - bug
branch: bug/fix-opencode-mcp-file-token-headers
worktree_path: .worktrees/fix-opencode-mcp-file-token-headers
acceptance_criteria:
    - An {env:NAME} token expands as before (loadConfig text pass) — existing tests unchanged.
    - 'In `mcp` remote headers, every embedded {file:PATH} token expands to the file''s content with surrounding whitespace trimmed: "Authorization: Bearer {file:...}" becomes the real credential.'
    - In `mcp` local `environment` values, embedded {file:PATH} tokens expand the same way.
    - A missing or unreadable file expands to an empty string ("Bearer ") and never fails the load.
    - File content is inserted after JSON parsing, so quotes/backslashes in a key file cannot break config load.
    - '`options.apiKey` semantics are unchanged (whole-value token, object forms); all existing opencode tests stay green.'
    - Key material never reaches logs, errors or panics.
    - doc/opencode.md and CHANGELOG.md describe the new behavior.
    - 'Scoped gates green: go build/test ./internal/opencode/..., make fmt-check on changed files, one scoped golangci-lint run.'
verification_plan:
    - 'go test ./internal/opencode/... with new table tests: embedded token in a header, token in an environment value, missing file → empty, content with quotes/backslashes stays intact, apiKey tests unchanged'
    - make fmt-check on changed files, one scoped golangci-lint run in the worktree
    - 'Manual: cozyphi with the live mcp-gateway config no longer sends the literal token (no 401)'
created_at: "2026-09-04T13:08:56.946129Z"
updated_at: "2026-09-04T13:09:55.570704Z"
---

## Body

**Symptom** A remote MCP server imported from opencode.json (`mcp-gateway`) carries `Authorization: Bearer {file:/Users/.../beeline-ai-api-key}`. CozyPhi sends the header literally and the gateway answers 401.

**Root cause** `loadConfig` in internal/opencode/source.go runs only the `{env:NAME}` text pass (envToken regex) over the raw config; `{file:PATH}` is expanded nowhere in the MCP path — `resolveServers` copies `mcpConfig.Headers`/`Environment` verbatim. The keySource file handling added for `options.apiKey` never touches MCP settings.

**Fix** After JSON parsing, expand embedded `{file:PATH}` tokens (same fileToken regex) in `mcp` remote `headers` and local `environment` values: each token becomes the file's content with surrounding whitespace trimmed; missing or unreadable file yields an empty string and never fails the load. Expansion happens on the parsed strings, so file content with quotes/backslashes cannot corrupt the JSON parse. Reuse the injected file reader (keySource.readFile or an explicit readFile func) so resolveServers stays testable; do not add a global {file:} text pass. Keep `options.apiKey` behavior unchanged (whole-value token and object forms, as landed in fix-opencode-config-only-providers).

**Out of scope** `{prompt:...}` interactive references, project-level opencode configs, auth.json, remote URL expansion.

**Docs** doc/opencode.md: the {file:} reference works in MCP headers and environment values (embedded allowed); an apiKey reference is still recognised only as a whole value. CHANGELOG.md entry under `## [Unreleased]`.

## Acceptance Criteria

- An {env:NAME} token expands as before (loadConfig text pass) — existing tests unchanged.
- In `mcp` remote headers, every embedded {file:PATH} token expands to the file's content with surrounding whitespace trimmed: "Authorization: Bearer {file:...}" becomes the real credential.
- In `mcp` local `environment` values, embedded {file:PATH} tokens expand the same way.
- A missing or unreadable file expands to an empty string ("Bearer ") and never fails the load.
- File content is inserted after JSON parsing, so quotes/backslashes in a key file cannot break config load.
- `options.apiKey` semantics are unchanged (whole-value token, object forms); all existing opencode tests stay green.
- Key material never reaches logs, errors or panics.
- doc/opencode.md and CHANGELOG.md describe the new behavior.
- Scoped gates green: go build/test ./internal/opencode/..., make fmt-check on changed files, one scoped golangci-lint run.

## Verification Plan

1. go test ./internal/opencode/... with new table tests: embedded token in a header, token in an environment value, missing file → empty, content with quotes/backslashes stays intact, apiKey tests unchanged
2. make fmt-check on changed files, one scoped golangci-lint run in the worktree
3. Manual: cozyphi with the live mcp-gateway config no longer sends the literal token (no 401)
