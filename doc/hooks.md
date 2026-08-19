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

One **plugin** is one directory with a `plugin.json` plus its scripts. Phi
loads every such directory under the hooks root (one level only — nested
folders are ignored). An optional `plugin.json` directly in the hooks root is
for a single ad-hoc plugin; with more than one plugin, use subdirectories.

```text
~/.phi/hooks/                    # user (lower)
  org-policy/
    plugin.json
    guard.sh
    audit.py
  secrets-scan/
    plugin.json
    scan.py

<cwd>/.phi/hooks/                # project (higher; same hook name replaces user)
  guard-bash/
    plugin.json
    run.sh
```

| Scope | Path | Precedence |
| --- | --- | --- |
| User | `~/.phi/hooks/<plugin>/plugin.json` (and optional `~/.phi/hooks/plugin.json`) | Lower |
| Project | `<cwd>/.phi/hooks/<plugin>/plugin.json` (and optional `<cwd>/.phi/hooks/plugin.json`) | Higher — same hook `name` replaces the user hook entirely |

- Phi creates an empty `~/.phi/hooks/` on startup if needed.
- `run` paths are relative to the directory that contains that `plugin.json`.
- Missing `plugin.json` is fine. Parse errors produce warnings and do not block startup.
- Duplicate hook names in the same scope: first definition wins (root file, then subdirs in filesystem order); later files warn and skip.
- Set `PHI_HOOKS=off` to disable discovery and execution entirely.

---

## Getting started

### 1. Create a project plugin

```text
.phi/hooks/guard-bash/
  plugin.json
  run.sh
```

**`plugin.json`**

```json
{
  "hooks": [
    {
      "name": "guard-bash",
      "event": "pre_tool",
      "match": "bash",
      "run": "./run.sh",
      "timeout": "5s",
      "fail_closed": true
    }
  ]
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

### Manifest (`plugin.json`)

A file is either `{"name":"plugin-id","hooks":[…]}` or a top-level `[…]` array of hook objects. `run` is relative to the directory that contains `plugin.json` (or absolute).

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `name` (plugin) | string | no | directory name | Optional plugin id |
| `hooks` | array | yes* | — | Hook entries (`*` not needed for a top-level array) |
| `name` (hook) | string | yes† | plugin `name` | Unique id; used for user/project override. †Optional only when the file has exactly one hook and the plugin has a name |
| `event` | string | yes | — | `pre_tool`, `post_tool`, or `command` |
| `match` | string | no | `*` | Exact tool name, or `*` for all tools. Not a regex. Ignored for `command`. |
| `run` | string | yes | — | Executable path relative to `plugin.json`'s directory, or absolute. Executed directly (no shell). |
| `timeout` | string \| number | no | `5s` | Go duration string (e.g. `"5s"`) or seconds as a number. Maximum `60s`. |
| `fail_closed` | boolean | no | `false` | On failure, deny (Pre) / stop (Post) instead of ignoring. Invalid on `command`. |
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
| `output` | Rewrites tool result for the model **and** TUI Output. Among sync hooks that set it, the last matching hook in entry order wins (execution is parallel, but the merge is deterministic) — prefer one rewrite hook. Not subject to the 4 KiB context cap. |
| `stop` / `reason` | Reserved stop signal (not yet wired into the agent loop). |

`async: true` hooks are fire-and-forget: their stdout is ignored, so they cannot contribute `context` or `output`.

| Exit code | Behavior |
| --- | --- |
| `0` | Parse stdout; empty body → no-op |
| `2` | Treated as stop request |
| other | Hook error → fail-open skip, or stop if `fail_closed` |

### Command (`event: "command"`)

A `command` hook registers a TUI slash command named after the hook `name` (leading `/` stripped, lowercased; must be one token). `/review` runs that hook's `run` script. Hook names are unique across all events — a `command` named `audit` replaces a `pre_tool` named `audit`. Builtin slash names (`sessions`, `resume`, `clear`, …) are not overwritten.

`async` and `fail_closed` are invalid. `match` is ignored.

stdin:

```json
{ "session_id": "…", "cwd": "/path/to/project", "hook_event": "command", "command": "review", "args": ["the", "diff"] }
```

stdout (first JSON line). Empty body + exit `0` is a silent success:

```json
{ "submit": "optional text sent as a user message" }
{ "toast": "optional success toast" }
```

| Exit code | Behavior |
| --- | --- |
| `0` | Parse stdout; empty body → no-op |
| other | Error toast (`reason` from JSON if present) |

The TUI runs at most one hook command at a time (like `!` bash). Reload drops in-flight results.

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

External hooks use a single JSON line on stdin and a single JSON line on stdout. Working directory is the directory that contains `plugin.json`. stdout/stderr are capped at **1 MiB** each. Aggregated model context from hooks is capped at **4 KiB**.

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

| Field | PreTool | PostTool | Command |
| --- | --- | --- | --- |
| `session_id` | yes | yes | yes |
| `cwd` | yes | yes | yes |
| `hook_event` | `pre_tool` | `post_tool` | `command` |
| `tool` | yes | yes | — |
| `tool_use_id` | yes | yes | — |
| `input` | yes | yes | — |
| `output` | — | tool stdout / result text when present | — |
| `error` | — | tool error text; empty on success | — |
| `command` | — | — | hook name |
| `args` | — | — | slash args after `/name` |

### Environment

Sensitive parent environment keys are stripped before spawn (substring match, case-insensitive), including patterns such as `API_KEY`, `SECRET`, `TOKEN`, `PASSWORD`, `PHI_API_KEY`, and common cloud credential names.

Injected variables:

| Variable | Value |
| --- | --- |
| `PHI_HOOK_EVENT` | `pre_tool`, `post_tool`, or `command` |
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
| Override a user hook | Declare the same hook `name` under `<cwd>/.phi/hooks/<plugin>/plugin.json` |

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
| `internal/hooks/` | Types, Manager, discovery (`plugin.json`), CommandHook, Load |
| `internal/agent/executor.go` | Pre → Gate → Run → Post |
| `internal/project` | `HooksDir()`, directory bootstrap |
| `internal/tui` | Engine wiring; list / reload; `HookCommands` registers slash commands |
