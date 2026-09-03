# MCP

cozyphi connects to MCP the **mcptoon way**: configure as many servers as you want; **tool schemas never enter the model context**.

That is the main difference from hosts that dump every `tools/list` schema into the prompt — ten or a hundred servers will not burn tens of thousands of tokens before you ask a question.

| Audience | This document |
| --- | --- |
| Users | Configure servers, CLI, how to call from the TUI |
| Contributors | Interaction flow and code map |

---

## Why this is a highlight

| Pain | Typical MCP host | cozyphi |
| --- | --- | --- |
| Context | All schemas injected at startup | Model sees only three meta-tools |
| Many servers | Uninstall / reload Tetris | Always configured; call on demand |
| Permissions | Separate system or wide open | Same Gate / Ask / Hooks as builtins |
| Footprint | Heavy SDKs, always-on processes | Hand-rolled Go stdio client, lazy start |

Agent-facing tools:

- `mcp_list` — list tool **names** on one server (compact text)
- `mcp_inspect` — slim parameter summary for one tool
- `mcp_call` — actually invoke

Configured **server names** are listed in the system prompt (like Skills), so the model knows what exists without calling `mcp_list` first. Schemas still stay out of context.

Typical rhythm: pick a server from the prompt → `mcp_list(server=…)` → `mcp_inspect` → `mcp_call`.

---

## Interaction flow

```text
Start TUI / cozyphi run
  → read enabled OpenCode servers (lowest priority)
  → overlay ~/.cozyphi/mcp.json + <cwd>/.cozyphi/mcp.json
  → build Pool (no subprocess yet)
  → tool list += mcp_list / mcp_inspect / mcp_call
  → system prompt += MCP catalog (server names only)

User prompt
  → model may call mcp_list(server=…) directly from the catalog
  → lazy Client → spawn → initialize → tools/list → names only
  → mcp_inspect → compact param summary
  → mcp_call → Executor → PreHooks → Gate → tools/call → result to model
```

Human CLI and the agent share the same `internal/mcp` stack:

```text
cozyphi mcp doctor|call  ──┐
                       ├──► Pool ──► Client (stdio JSON-RPC)
model mcp_* ───────────┘
```

Sub-agents do **not** inherit MCP meta-tools by default. Disable with `COZYPHI_MCP=off`.

---

## OpenCode source

When `opencode.enabled` is absent or `true`, CozyPhi also reads enabled MCP
servers directly from OpenCode's global `opencode.json`. Local servers become
stdio entries and remote servers become HTTP entries. Remote servers requiring
OAuth are skipped because CozyPhi must not modify OpenCode's OAuth state.

OpenCode server names are imported unchanged. A same-named server in
`~/.cozyphi/mcp.json` or the project MCP config overrides the imported entry.
No OpenCode data is copied into `~/.cozyphi`; restart CozyPhi after changing the
setting or either MCP configuration. See [OpenCode integration](opencode.md).

---

## Quick start

Config file: `~/.cozyphi/mcp.json` (project `<cwd>/.cozyphi/mcp.json` overrides same-named servers).

```json
{
  "servers": {
    "browsermcp": {
      "transport": "stdio",
      "command": ["npx"],
      "args": ["@browsermcp/mcp@latest"],
      "timeout": "5m"
    },
    "remote": {
      "transport": "http",
      "url": "http://127.0.0.1:3001/mcp",
      "headers": { "Authorization": "Bearer …" },
      "timeout": "300s"
    }
  }
}
```

`timeout` (optional) sets the per-call timeout for that server. It accepts Go duration syntax (`"30s"`, `"5m"`). Default: `60s`.
```


### Permissions

`mcp_call` runs another program's code, so it goes through the same Gate as
builtins: every call **asks** by default, naming the server and tool. To
pre-approve a server or a single tool, list it in `~/.cozyphi/config.yaml`:

```yaml
permissions:
  mcp:
    allow:
      - '^fetch/'
      - '^github/create_issue$'
```

Entries are regexes matched against `server/tool`. Allowlisted calls skip the
ask — which is what makes MCP usable in `cozyphi run` (headless) and
sub-agents, where nobody can answer an ask and every ask folds to a denial.
Unlisted servers keep asking interactively.

Or via CLI:

```sh
cozyphi mcp add browsermcp -- npx @browsermcp/mcp@latest
cozyphi mcp list
cozyphi mcp doctor
```

**Restart cozyphi** after config changes (Pool loads at startup).

### Migrating from Claude Desktop config

Claude / Cursor style:

```json
{
  "mcpServers": {
    "browsermcp": {
      "command": "npx",
      "args": ["@browsermcp/mcp@latest"]
    }
  }
}
```

cozyphi equivalent:

```json
{
  "servers": {
    "browsermcp": {
      "transport": "stdio",
      "command": ["npx"],
      "args": ["@browsermcp/mcp@latest"]
    }
  }
}
```

`mcpServers` → `servers`; string `command` becomes the first element of the `command` array.

---

## CLI

```text
cozyphi mcp list                         list configured servers
cozyphi mcp add <name> -- <cmd> [args…]  write ~/.cozyphi/mcp.json
cozyphi mcp remove <name>                remove from user config
cozyphi mcp call <server> <tool> [json]  call a tool directly
cozyphi mcp doctor                       check config + connectivity
```

Logs: `~/.cozyphi/logs/mcp/<name>.log` (override with `COZYPHI_MCP_LOG_DIR`).
A server's stderr log is capped at 1 MiB and created 0600; past the cap the
file is rewritten with the newest output alone, so it holds the most recent
failure rather than the first one.

---

## Limits (v1)

- Transports: **stdio** and **http** (POST JSON / SSE `data:` bodies, `Mcp-Session-Id`)
- One JSON-RPC frame is capped at **1 MiB** on stdio; a larger frame fails the
  call instead of growing the reader without bound
- MCP tools are not registered individually into the model tool list (by design)
- Dead subprocesses reconnect on the next call; no elaborate self-heal state machine
- Some third-party packages may crash on start — use `doctor` + logs.

---

## Related code

| Path | Role |
| --- | --- |
| `internal/mcp/` | config, Client, session, stdio/http transports, Pool |
| `internal/tools/mcptool/` | `mcp_list` / `mcp_inspect` / `mcp_call` |
| `internal/agent/engine.go` | `EngineOpts.MCP` wires meta-tools |
| `cmd/mcp.go` | `cozyphi mcp` subcommand |
