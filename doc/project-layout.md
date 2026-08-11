# Project layout

| Path                     | Purpose                                        |
| ------------------------ | ---------------------------------------------- |
| `cmd/`                   | Entry points (`main.go`, `phi run`, `phi update`, `phi sessions`) |
| `internal/util/update/`  | Self-update check + GitHub Releases install    |
| `internal/agent/`        | Agent engine, executor, prompts                |
| `internal/components/`   | TUI widgets (chat, input, palette, mention, …) |
| `internal/llm/`          | LLM clients (OpenAI-compatible + Anthropic), streaming, skills |
| `internal/project/`      | Workspace layout and config                    |
| `internal/session/`      | Session persistence, load/apply                |
| `internal/job/`          | Sub-agent job manager (spawn/wait/cancel)      |
| `internal/tools/`        | Agent tools (`*tool` packages + `tooldef`)     |
| `internal/toolmanager/`  | External tool discovery/download               |
| `internal/tui/`          | Terminal UI wiring: controller, commands, keymaps |
| `internal/util/`         | Shared helpers (diff, retry, SSE, file search, …) |
| `internal/permission/`   | Permission policy and ask gate                 |
| `internal/hooks/`        | Tool-loop hooks (discover, Manager, CommandHook) |

## Design docs

| Path | Purpose |
| ---- | ------- |
| [`hooks.md`](hooks.md) | Hooks: concepts, authoring, protocol reference |
