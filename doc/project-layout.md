# Project layout

| Path                     | Purpose                                        |
| ------------------------ | ---------------------------------------------- |
| `cmd/`                   | Entry points (`main.go`, `cozyphi run`, `cozyphi update`, `cozyphi sessions`, `cozyphi memory`) |
| `internal/util/update/`  | Self-update check + GitHub Releases install    |
| `internal/agent/`        | Agent engine, executor, jobs                     |
| `internal/agent/prompt/` | System prompt templates + Skills/MCP catalogs    |
| `internal/components/`   | TUI widgets (chat, input, palette, mention, …) |
| `internal/llm/`          | LLM clients (OpenAI-compatible + Anthropic), streaming, skills |
| `internal/lsp/`          | Harness-managed gopls client, one model-facing query seam |
| `internal/memory/`       | Agent memory: fact files, generated index, per-turn recall |
| `internal/project/`      | Workspace layout and config                    |
| `internal/session/`      | Session persistence, load/apply                |
| `internal/job/`          | Sub-agent job manager (spawn/wait/cancel)      |
| `internal/tools/`        | Agent tools (`*tool` packages + `tooldef`)     |
| `internal/toolmanager/`  | External tool discovery/download               |
| `internal/tui/editor/`   | TUI root widget (`Editor`), layout, dispatch, branch watch |
| `internal/tui/transcript/` | Session→widget projection (Mapper, Pane) |
| `internal/tui/composer/` | Chat input, slash/@ pickers, palette |
| `internal/tui/footer/`   | Activity spinner, token labels, update hint |
| `internal/tui/overlays/` | Permission / continue-ask panels |
| `internal/tui/submit/`   | Submit, cancel, slash dispatch, bash runner |
| `internal/tui/commands/` | Slash/palette registry, session/hook commands |
| `internal/tui/pathutil/` | Cwd + git branch path labels |
| `internal/tui/controller/` | Engine lifecycle, Bus/Msg, activity |
| `internal/version/`      | Build-time `Version` (splash / `cozyphi update`) |
| `internal/util/`         | Shared helpers (diff, retry, SSE, file search, …) |
| `internal/permission/`   | Permission policy and ask gate                 |
| `internal/hooks/`        | Tool-loop hooks (`plugin.json`, Manager, CommandHook) |
| `internal/mcp/`          | MCP config + stdio client + pool (meta-tool route) |

## Design docs

| Path | Purpose |
| ---- | ------- |
| [`hooks.md`](hooks.md) | Hooks: concepts, authoring, protocol reference |
| [`mcp.md`](mcp.md) | MCP: zero schema pollution, meta-tools, config, CLI |
| [`tui.md`](tui.md) | TUI: package layout, aggregation, interaction flows |
| [`lsp.md`](lsp.md) | LSP: config, operations, lifecycle, limits |
| [`memory.md`](memory.md) | Memory: file format, index, recall, code map |
