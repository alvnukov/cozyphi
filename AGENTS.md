# Phi

Minimal Go terminal coding-agent harness. Layout: [doc/project-layout.md](doc/project-layout.md). Humans: [CONTRIBUTING.md](CONTRIBUTING.md).

## Communication Preferences

- Dry, concise, low-key humor. No flattery, no forced memes. Skip preambles and postambles.
- Comments explain "why", not "what". English only — this repo was migrated off Chinese comments.
- Error messages: actionable and specific. No vague "something went wrong".

## Constraints

- **Tool loop is PreHooks → Gate/Ask → Run → PostHooks.** Don't bypass the permission gate. Don't put MCP server tool schemas on the model — only `mcp_list` / `mcp_inspect` / `mcp_call`.
- **Hashline `edit` is LINE#HASH.** Stale hashes fail closed. Don't "simplify" to whole-file rewrite.
- **Sub-agent transcripts stay under `~/.phi/jobs/<id>/`.** Parent context gets the wait/task summary only. Child engines have no `agent_*` tools (no nesting). Default child role is explore (read-only).
- **UI split:** `internal/components` render; `internal/tui` wires. Keep widgets dumb.
- **Stay lean.** Direct module deps are few on purpose. Don't add a dependency without a clear need.
- **Format with `make fmt`** (gofumpt / goimports / golines, 120 cols, local prefix `github.com/pulseaiclub/phi`). Don't hand-fight import groups.
- **`testing` / `testify` stay in `*_test.go`.** `depguard` will fail the lint otherwise.
- After dependency changes: `go mod tidy`. `go.mod` is not generated.

## Contributor Guidelines

- Keep changes focused and reviewable. Add or update tests next to the code.
- Conventional Commits, English, lowercase, imperative, ≤72 chars. One logical change per commit.
- Do not put `@mentions` or `fixes #...` in commit messages (those belong in the PR).
- Do not add `Co-authored-by:`.

## Commands

```
make help
make test                      # go test ./...
go test ./internal/hooks -v    # one package
make fmt                       # apply formatters
make fmt-check                 # CI formatting gate
make lint                      # golangci-lint
make build                     # ./phi
```

## Style

- Packages: lowercase, single word, match the directory (`writetool`, not `write_tool`).
- Prefer small packages under `internal/`; keep the exported surface small.
- Tests live beside the code they cover.
