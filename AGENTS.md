# CozyPhi

A comfortable, feature-rich terminal coding agent in Go — forked from
alvnukov/cozyphi, taking the best from Claude Code, opencode and friends.
Phase 1: UI/UX convenience. Phase 2: replace built-in tools with cozy-tools,
a library extracted from mcp-ai-helper.

Layout: [doc/project-layout.md](doc/project-layout.md). Design docs:
`doc/hooks.md`, `doc/mcp.md`, `doc/tui.md`, `doc/memory.md`,
`doc/watch.md`.

## Quality bar

Every change is weighed on six axes; when they conflict, trade them off out loud.

- **Architecture** — deep modules: much behaviour behind a small interface, so
  a change lands in one place. Consolidate into one deep module rather than
  adding pass-through layers. Packages: lowercase, single word, matching the
  directory; small exported surface.
- **Extensibility** — variation goes behind a seam, not into callers. A seam
  with two adapters is real; with one, it is hypothetical.
- **Testability** — tests exercise the public interface and live beside the
  code they cover. `testing`/`testify` stay in `*_test.go` (depguard).
- **Security** — the permission gate asks by default; only an explicit user
  flag opts out. Secrets never reach logs, panics, or the transcript. Tool
  boundaries validate input (paths, commands) before acting.
- **Reliability** — errors are wrapped, checked, and actionable; cancellation
  is honored; goroutines, channels and files always get an exit.
- **Readability** — the code is read by humans and agents: English only,
  comments explain why, names say the thing, error messages say what to do.

## Invariants

- **Tool loop:** PreHooks → Gate/Ask → Run → PostHooks. The executor never
  bypasses the permission gate. MCP servers reach the model only as
  `mcp_list` / `mcp_inspect` / `mcp_call` — server tool schemas stay
  off-context.
- **Hashline `edit`:** edits anchor on `@file path#TAG` / `LINE#HASH`; stale
  anchors fail closed. Never swap it for whole-file rewrite.
- **Sub-agents:** transcripts stay under `~/.cozyphi/jobs/<id>/`; the parent gets
  the wait/task summary only; child engines carry no `agent_*` tools; default
  child role is explore (read-only).
- **UI split:** `internal/components` render; `internal/tui` wires the shell —
  non-shell pieces under `internal/tui/controller` (Engine/Bus/Msg),
  `internal/tui/transcript` (Mapper), `internal/version`. Widgets stay dumb;
  rendering is draw-on-demand — widgets wake the scheduler through
  `DrawContext.Wake`/`WakeIn`/`WakeAt`, idle frames write zero bytes.
- **Assembly:** `cmd` constructs Bus/Controller/App/registry and passes them
  into `editor.NewEditor(...)`. Constructors take parameters (never
  `XxxDeps` bags), return fully initialized objects, and keep
  `GetDefaultProject` out of `tui`.
- **Memory:** facts live under `~/.cozyphi/memory/<encoded-cwd>/`, one file per
  fact, written with `write`. The harness generates `MEMORY.md` and renders the
  prompt from the files; the agent never edits the index. Kind decides reach:
  `user`/`feedback` ride in the system prompt, `project`/`reference` are
  retrieved for the turn that matches them. Every tier is capped, and retrieval
  costs posting lists, not files. Nothing is ever deleted — the harness demotes
  no further than "findable but unlisted", and `forget` moves a file into
  `forgotten/`; `pin: true` is never demoted. The `memory` tool reads, prunes
  and never writes a fact. Sub-agents get no store, and so no tool.
- **Watches:** a watch is a background shell command whose output wakes the
  session. Starting one is judged by the bash deny list and default — never the
  bash allowlist, which clears a command to run once, not forever. Events reach
  the user as a transcript row and the model as a reminder block that names the
  watch; they are never a user message. Four bounds hold: 20 events a minute, 8 live watches, 5
  turns started in a row without user input, and process lifetime — nothing is
  persisted. Sub-agents and headless runs get no manager, and so no tool.
- **Deps stay lean:** a new direct dependency needs a clear need;
  `go mod tidy` after dependency changes — `go.mod` is hand-maintained.

## Working here

- Format with `make fmt` (gofumpt/golines, 120 cols) — hand-fighting import
  groups loses to the formatters. CI gates: `make fmt-check lint test`.
- Conventional Commits: English, lowercase, imperative, ≤72 chars, one
  logical change per commit; `@mentions`, `fixes #…` and `Co-authored-by:`
  stay out of commit messages.
- User-visible changes add a line under `## [Unreleased]` in `CHANGELOG.md`
  (CI protects the released section).
- Work is tracked in the mcp-ai-helper registry (`obsidian-tasks/`): found
  bugs become task-issues there; session notes go to
  `.mcp-ai-helper/notes/`.

## Communication

Dry, concise, low-key humor. Skip preambles and postambles.
