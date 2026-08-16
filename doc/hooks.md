# Hooks

Hooks let you run custom logic around each tool call—before permission gating and after execution—without changing Phi’s binary or putting settings into `config.yaml`.

Use hooks when you need organization policy, audit trails, or input rewriting that the permission Gate does not cover.

| Audience | This document |
| --- | --- |
| Hook authors | Create and test scripts under `.phi/hooks/` |
| Operators | Deploy user- or project-level policy |
| Contributors | See [Related code](#related-code) |

---

## Concepts

### Execution order

```text
emit(InProgress)
  → PreTool hooks     (allow | deny | modify)
  → Gate              (Ask UI / permission rules)
  → tool.Run
  → PostTool hooks    (optional context / output rewrite)
  → emit(Done | …)
```

- **PreTool** runs before Gate. A deny can stop a tool without user approval.
- **PostTool** can append model-facing `context` and/or rewrite the tool `output`.
  - `context` is wrapped in `<hook_context>…</hook_context>` on the tool result sent to the model only. TUI Detail/Output are unchanged by `context`. If no hook returns `context`, the tags are omitted.
  - `output` replaces both the model-facing tool content and the TUI Output string for that tool run (Detail is unchanged). Omit `output` (or leave it empty) to keep the original tool result.
- If no hooks are loaded, behavior matches a build with hooks disabled.

### Discovery model

Each hook is a directory containing a required `hook.json` and an executable referenced by `run`.

| Scope | Path | Precedence |
| --- | --- | --- |
| User | `~/.phi/hooks/<name>/` | Lower |
| Project | `<cwd>/.phi/hooks/<name>/` | Higher — same `<name>` replaces the user hook entirely |

- Phi creates an empty `~/.phi/hooks/` on startup if needed.
- Directories without a valid `hook.json` are skipped. Parse errors produce warnings and do not block startup.
- Set `PHI_HOOKS=off` to disable discovery and execution entirely.

---

## Getting started

### 1. Create a project hook

```text
.phi/hooks/guard-bash/
  hook.json
  run.sh
```

**`hook.json`**

```json
{
  "name": "guard-bash",
  "event": "pre_tool",
  "match": "bash",
  "run": "./run.sh",
  "timeout": "5s",
  "fail_closed": true
}
```

**`run.sh`** (must be executable: `chmod +x run.sh`)

```bash
#!/usr/bin/env bash
# Deny bash commands whose text contains "phi-deny".
input=$(cat)
case "$input" in
  *phi-deny*)
    echo '{"action":"deny","reason":"blocked by guard-bash (matched phi-deny)"}'
    exit 2
    ;;
esac
echo '{"action":"allow"}'
```

### 2. Load hooks

- Restart Phi, or
- Command palette: **hooks → reload** (`Ctrl+K`)

List loaded hooks with **hooks → list**.

### 3. Verify

Ask the agent to run `echo phi-deny`. The PreTool hook should deny the call.

---

## Authoring guide

### Manifest (`hook.json`)

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `name` | string | no | directory name | Unique id; used for user/project override |
| `event` | string | yes | — | `pre_tool` or `post_tool` |
| `match` | string | no | `*` | Exact tool name, or `*` for all tools. Not a regex. |
| `run` | string | yes | — | Executable path relative to the hook directory, or absolute. Executed directly (no shell). |
| `timeout` | string \| number | no | `5s` | Go duration string (e.g. `"5s"`) or seconds as a number. Maximum `60s`. |
| `fail_closed` | boolean | no | `false` | On failure, deny (Pre) / stop (Post) instead of ignoring |
| `async` | boolean | no | `false` | `post_tool` only: fire-and-forget; result ignored |
| `disabled` | boolean | no | `false` | Skip loading this hook |

### PreTool response

Write one JSON object on stdout (first line only). Empty stdout with exit `0` means allow.

```json
{ "action": "allow" }
{ "action": "deny", "reason": "policy violation" }
{ "action": "modify", "input": { "command": "echo safe" } }
```

| Exit code | Behavior |
| --- | --- |
| `0` | Parse stdout; empty body → allow |
| `2` | Hard deny (even with empty body) |
| other | Treated as hook error → fail-open skip, or deny if `fail_closed` |

Optional fields on success: `reason`, `context` (model-facing note).

### PostTool response

```json
{ "context": "note for the model", "output": "rewritten tool result", "stop": false, "reason": "" }
```

| Field | Effect |
| --- | --- |
| `context` | Model-only note (see Concepts). Aggregated from matching sync hooks (joined; capped at 4 KiB). |
| `output` | Rewrites tool result for the model **and** TUI Output. Aggregated the same way as `context` when several sync hooks return it — prefer a single rewrite hook per tool. |
| `stop` / `reason` | Reserved stop signal (not yet wired into the agent loop). |

`async: true` hooks are fire-and-forget: their stdout is ignored, so they cannot contribute `context` or `output`.

| Exit code | Behavior |
| --- | --- |
| `0` | Parse stdout; empty body → no-op |
| `2` | Treated as stop request |
| other | Hook error → fail-open skip, or stop if `fail_closed` |

### Failure policy (`fail_closed`)

| Value | When the script crashes, times out, or returns invalid JSON |
| --- | --- |
| `false` (default) | Ignore that hook (suitable for audit) |
| `true` | Deny (Pre) or stop (Post) (suitable for security gates) |

In `permissions.mode: readonly`, only hooks with `fail_closed: true` run, so slow audit hooks do not stall exploratory tool use. Interactive sessions and `phi run` run all loaded hooks.

### Ordering and concurrency

- Matching **PreTool** hooks run **serially**. First deny wins; modify results chain onto `input`.
- Matching **PostTool** hooks run **in parallel** (except `async`, which is detached).
- Order across multiple hooks is **not** guaranteed. If order matters, put the logic in one hook.
- Because PostTool runs in parallel, do not rely on several hooks each rewriting `output` for the same tool call; put rewrite logic in one sync hook.

---

## Protocol reference

External hooks use a single JSON line on stdin and a single JSON line on stdout. Working directory is the hook directory. stdout/stderr are capped at **1 MiB** each. Aggregated model context from hooks is capped at **4 KiB**.

### Request (stdin)

```json
{
  "session_id": "…",
  "cwd": "/path/to/project",
  "hook_event": "pre_tool",
  "tool": "bash",
  "tool_use_id": "call_…",
  "input": { "command": "ls" }
}
```

| Field | PreTool | PostTool |
| --- | --- | --- |
| `session_id` | yes | yes |
| `cwd` | yes | yes |
| `hook_event` | `pre_tool` | `post_tool` |
| `tool` | yes | yes |
| `tool_use_id` | yes | yes |
| `input` | yes | yes |
| `output` | — | tool stdout / result text when present |
| `error` | — | tool error text; empty on success |

### Environment

Sensitive parent environment keys are stripped before spawn (substring match, case-insensitive), including patterns such as `API_KEY`, `SECRET`, `TOKEN`, `PASSWORD`, `PHI_API_KEY`, and common cloud credential names.

Injected variables:

| Variable | Value |
| --- | --- |
| `PHI_HOOK_EVENT` | `pre_tool` or `post_tool` |
| `PHI_SESSION_ID` | Session id |
| `PHI_CWD` | Workspace cwd |
| `PHI_PROJECT_DIR` | Same as cwd for command hooks |

---

## Operations

| Action | How |
| --- | --- |
| Disable all hooks | `PHI_HOOKS=off` |
| Inspect load warnings | `PHI_DEBUG=1` |
| List / reload in TUI | `Ctrl+K` → **hooks → list** / **hooks → reload** |
| Override a user hook | Place a directory with the same `name` under `<cwd>/.phi/hooks/` |

Configuration for hooks is **not** stored in `~/.phi/config.yaml` or managed via `phi config`.

---

## Limitations

The following are intentionally out of scope:

- Long-lived plugin host processes or bidirectional RPC
- File-watch based hot reload (use palette reload or restart)
- Registering new tools from hooks (use `tooldef.Tool`)
- Mixing hook definitions into the main YAML config

---

## Related code

| Path | Role |
| --- | --- |
| `internal/hooks/` | Types, Manager, discovery, CommandHook, Load |
| `internal/agent/executor.go` | Pre → Gate → Run → Post |
| `internal/project` | `HooksDir()`, directory bootstrap |
| `internal/tui` | Engine wiring; list / reload commands |
