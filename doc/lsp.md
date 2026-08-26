# LSP

CozyPhi ships one model-facing tool, `lsp`, backed by a harness-managed
[gopls](https://go.dev/wiki/gopls) process. The model never starts, stops, or
configures a language server: it queries, the harness owns the lifecycle.

## Scope

V1 speaks Go only, through gopls only. There is no per-project server
configuration and no generic arbitrary-server adapter; a second real adapter
can introduce the seam.

## Configuration

The single global config is `~/.cozyphi/lsp.json`:

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

- A missing file means built-in defaults. A malformed, unknown-key,
  symlinked, non-regular, foreign-owned, or group/world-writable file fails
  closed with a sanitized error.
- `command` is non-empty argv launched without a shell. The first element is
  an absolute path or a bare basename resolved through `~/.cozyphi/bin` and
  then `PATH` — never the working directory.
- `env` adds to a sanitized inherited environment; `initialization_options`
  appears only in `initialize`; `settings` answers `workspace/configuration`
  and one `didChangeConfiguration` push.
- `enabled: false` registers no `lsp` tool at all. There is no
  environment-variable override in V1.
- Project-local `.cozyphi/lsp.json` is never read.

Install gopls with `go install golang.org/x/tools/gopls@latest`. A missing
binary never blocks startup: `op=languages` reports it with the install hint.

## Operations

`lsp` takes one `op` plus the fields it needs. Positions are 1-based Unicode
code points (line and character); results are workspace-relative paths with
1-based, end-exclusive ranges.

- `languages` — status: configured, installed, running, active roots,
  supported operations, install hint. Starts nothing.
- `definition` — file + (line, character) or symbol.
- `references` — same targeting, optional `includeDeclaration`.
- `hover` — same targeting; bounded markdown/plaintext.
- `symbols` — file (document symbols) or `query` (workspace-wide search).
- `calls` — call hierarchy; requires `direction` `incoming` or `outgoing`.
- `diagnostics` — file; merges push and pull diagnostics with a provenance
  status: `fresh`, `cached`, `unconfirmed`, or `pending`.

The harness synchronizes documents (didOpen/didChange with UTF-16 position
conversion, LRU with didClose) before position-sensitive queries, so results
match the disk state at query time.

## Lifecycle

- The initialize handshake and ordinary queries time out after 15 seconds;
  workspace-wide symbol search after 30. Graceful shutdown gets 2 seconds,
  then the whole process tree is killed and reaped.
- Canceling a query sends `$/cancelRequest` and never touches the shared
  process; late responses are discarded.
- A dead generation (EOF, malformed framing, process exit, writer failure)
  fails every pending request; a later query lazily starts a fresh generation.
  No query is ever transparently executed more than twice.
- A per-root circuit breaker allows three gopls starts per rolling 60-second
  window. Failed spawns and failed handshakes consume quota; config errors and
  a missing binary do not. The fourth start fails with a typed `unavailable`
  error carrying `retry_after_seconds`.
- Close order is exact: reject new queries, cancel pending, didClose open
  documents, `shutdown`, `exit`, close stdin and wait, then kill the tree.

## Errors and limits

Typed error kinds: `invalid`, `ambiguous`, `unsupported`, `unavailable`,
`protocol`, `closed`. Frozen resource limits: 8 KiB headers, 8 MiB frames,
4 MiB file reads, 32 open documents / 32 MiB synchronized text, 50 default /
200 hard item limit, 8 KiB text fields, 64 KiB stderr tail, 50 KiB rendered
output. Truncation is always reported; raw protocol payloads never enter
results, logs, or the transcript.

## Security and privacy

Every input path and every returned URI passes the same physical-containment
seam (symlinks resolved, workspace-relative only). Only local `file://` URIs
are accepted. Mutating server requests are declined: `applyEdit` answers
`applied: false`, dynamic registration is refused. argv and env come from the
owner's config alone and are redacted from every error and result. Note the
honest boundary: trusted gopls has normal OS-level workspace access — the
per-tool permission gate governs the model's queries, not gopls syscalls.

## Troubleshooting

- `gopls executable not found` — install gopls or set `gopls.command` in
  `~/.cozyphi/lsp.json`.
- `unavailable` with `retry_after_seconds` — gopls crashed repeatedly; wait
  out the cool-down and retry once.
- Stale results after a crash — generations never serve cached state across a
  restart; rerun the query.
- No `lsp` tool at all — `enabled` is false in `~/.cozyphi/lsp.json`.
