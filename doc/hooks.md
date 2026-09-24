# Hooks

Hooks let you run custom logic around each tool call—before permission gating and after execution—without changing CozyPhi’s binary or putting settings into `config.yaml`.

Use hooks when you need organization policy, audit trails, or input rewriting that the permission Gate does not cover.

| Audience | This document |
| --- | --- |
| Hook authors | Create and test scripts under `.cozyphi/hooks/` |
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

One **hook manifest** is one directory with a `plugin.json` plus its scripts.
(The file name stays `plugin.json` for compatibility; the directory it
describes is called a hook manifest here to keep it apart from a Claude Code
plugin — see "Claude Code plugin hooks" below, a separate, independent
source.) CozyPhi loads every such directory under the hooks root (one level
only — nested folders are ignored). An optional `plugin.json` directly in the
hooks root is for a single ad-hoc hook manifest; with more than one, use
subdirectories.

```text
~/.cozyphi/hooks/                    # user (lower)
  org-policy/
    plugin.json
    guard.sh
    audit.py
  secrets-scan/
    plugin.json
    scan.py

<cwd>/.cozyphi/hooks/                # project (higher; same hook name replaces user)
  guard-bash/
    plugin.json
    run.sh
```

| Scope | Path | Precedence |
| --- | --- | --- |
| User | `~/.cozyphi/hooks/<name>/plugin.json` (and optional `~/.cozyphi/hooks/plugin.json`) | Lower |
| Project | `<cwd>/.cozyphi/hooks/<name>/plugin.json` (and optional `<cwd>/.cozyphi/hooks/plugin.json`) | Higher — same hook `name` replaces the user hook entirely |

- CozyPhi creates an empty `~/.cozyphi/hooks/` on startup if needed.
- `run` paths are relative to the directory that contains that `plugin.json`.
- A missing `plugin.json` is fine. Parse errors produce warnings and do not block startup.
- Duplicate hook names in the same scope: first definition wins (root file, then subdirs in filesystem order); later files warn and skip.
- Set `COZYPHI_HOOKS=off` to disable discovery and execution entirely — for
  hook manifests and Claude Code plugin hooks alike. (`COZYPHI_PLUGINS=off`
  is the separate switch that stops cozyphi from finding Claude Code plugins
  at all; see `doc/plugins.md`.)

---

## Getting started

### 1. Create a project hook manifest

```text
.cozyphi/hooks/guard-bash/
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
# Deny bash commands whose text contains "cozyphi-deny".
input=$(cat)
case "$input" in
  *cozyphi-deny*)
    echo '{"action":"deny","reason":"blocked by guard-bash (matched cozyphi-deny)"}'
    exit 2
    ;;
esac
echo '{"action":"allow"}'
```

### 2. Load hooks

- Restart CozyPhi, or
- Command palette: **hooks → reload** (`Ctrl+K`)

List loaded hooks with **hooks → list**.

### 3. Verify

Ask the agent to run `echo cozyphi-deny`. The PreTool hook should deny the call.

---

## Authoring guide

### Manifest (`plugin.json`)

A file is either `{"name":"plugin-id","hooks":[…]}` or a top-level `[…]` array of hook objects. `run` is relative to the directory that contains `plugin.json` (or absolute).

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `name` (manifest) | string | no | directory name | Optional hook-manifest id |
| `hooks` | array | yes* | — | Hook entries (`*` not needed for a top-level array) |
| `name` (hook) | string | yes† | manifest `name` | Unique id; used for user/project override. †Optional only when the file has exactly one hook and the manifest has a name |
| `event` | string | yes | — | `pre_tool`, `post_tool`, `post_turn`, `command`, `session_start`, `session_shutdown`, or `session_before_switch` |
| `match` | string | no | `*` | Exact tool name, or `*` for all tools. Not a regex. Ignored for `command` and session events. |
| `run` | string | yes | — | Executable path relative to `plugin.json`'s directory, or absolute. Executed directly (no shell). |
| `timeout` | string \| number | no | `5s` | Go duration string (e.g. `"5s"`) or seconds as a number. Maximum `60s`. |
| `fail_closed` | boolean | no | `false` | On failure, deny (Pre / before_switch) / stop (Post). Invalid on `command`, `session_start`, `session_shutdown`. |
| `async` | boolean | no | `false` | `post_tool` / `post_turn` / `session_start` / `session_shutdown`: fire-and-forget; result ignored |
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
| `stop` / `reason` | Stops the run: the round's remaining tool calls do not execute, the agent loop ends with the reason surfaced to the user, and the stopped call's result tells the model why. |

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

stdout (first JSON line). Empty body + exit `0` is a silent success.

Apply order after a successful run: **status** → **toast** → **list** (palette page) → **submit** (skipped when `list` is present).

```json
{ "submit": "optional text sent as a user message" }
{ "toast": "optional success toast" }
{ "status": "footer status text" }
{ "status": "" }
{ "list": { "title": "Findings", "items": [{ "label": "auth.go:12", "detail": "nil check", "submit": "fix auth.go:12" }] } }
```

| Field | Effect |
| --- | --- |
| `submit` | Send as a user message (ignored when `list` is set) |
| `toast` | Success toast |
| `status` | Set footer status when the key is present (empty string clears) |
| `list` | Push a Ctrl+K palette page; item `submit` runs on select |

| Exit code | Behavior |
| --- | --- |
| `0` | Parse stdout; empty body → no-op |
| other | Error toast (`reason` from JSON if present) |

The TUI runs at most one hook command at a time (like `!` bash). Reload drops in-flight results.

### Session lifecycle

| Event | When | Can block? |
| --- | --- | --- |
| `session_before_switch` | Before `/clear` or `/resume` replaces the engine | Yes — `action: deny` or exit `2` |
| `session_shutdown` | Leaving a session (`new` / `resume` / `quit`) | No |
| `session_start` | After a session is ready (`startup` / `new` / `resume`), and again after every successful compaction (`compact`) | No |

`async: true` is allowed on `session_start` and `session_shutdown` (fire-and-forget). `fail_closed` is allowed only on `session_before_switch`. `match` is ignored.

`resume` also covers a session opened at launch with `--resume`/`--continue` and a fork opened into a new tab — both start from an existing transcript, so neither re-runs the `startup` bootstrap.

`compact` fires `session_start` again after every successful compaction —
automatic overflow recovery, manual `/compact`, the compaction the model
requests through the `context` tool, and a successful user trim of the
context (the `/context` browser's trim action) — so a `session_start` hook
that set something up (a plugin bootstrap, but any `session_start` hook sees
it) runs again once the history it depended on has been summarized or cut.
It fires only in the primary engine, never in a sub-agent. This is new:
existing `session_start` hooks now see a reason they did not before, so a
hook that switches on `reason` should treat an unrecognized value as a no-op
rather than an error.

stdin:

```json
{
  "session_id": "…",
  "cwd": "/path/to/project",
  "hook_event": "session_before_switch",
  "reason": "resume",
  "target_session_id": "abcd…",
  "usage": { "prompt_tokens": 12, "completion_tokens": 7, "total_tokens": 19 }
}
```

| Field | Meaning |
| --- | --- |
| `reason` | `startup` \| `new` \| `resume` \| `compact` \| `quit` — `compact` reaches `session_start` only |
| `previous_session_id` | On `session_start` after a switch: the session just left |
| `target_session_id` | On `session_before_switch` for resume: destination id |
| `usage` | Token usage of the latest completed assistant turn (see below) |

stdout examples:

```json
{ "action": "deny", "reason": "uncommitted changes" }
{ "toast": "session ready", "status": "hooks on" }
```

`session_before_switch` runs **serially** (first deny wins). Start/shutdown run **in parallel** (like PostTool). Toast/status from session hooks are applied by the TUI when present.

`usage` reports the token usage of the most recent completed assistant turn: `prompt_tokens`, `completion_tokens`, `cached_tokens` (prompt cache reads), and `total_tokens`, each omitted when zero. The value comes from the live stream, not the session file: `session_start` sees an empty usage (a just-started or resumed session has no completed turn yet in this process), while `session_shutdown` and `session_before_switch` carry the last turn of the session being left. A cancelled or errored turn does not overwrite the previous value.

### Post-turn (`post_turn`)

Fires after each completed assistant stream in the interactive TUI run loop
(Controller.recordUsage). stdin matches session lifecycle fields: `session_id`, `cwd`, `message_id`, and `usage`. `async: true` is recommended so slow loggers do not stall the agent loop. `fail_closed` is not valid. Results are audit-only — stdout is not injected into the model or transcript.

`message_id` is the id of the assistant entry in the session file, the same id
the transcript row carries, so a hook can line its log up against the session
afterwards. It is an opaque token; earlier versions shaped it as
`assistant-<nanos>`, and nothing should parse it.

Example stdin:

```json
{
  "session_id": "…",
  "cwd": "/path/to/project",
  "hook_event": "post_turn",
  "message_id": "b3f1a0274c9d6e15",
  "usage": { "prompt_tokens": 1200, "cached_tokens": 900, "completion_tokens": 40, "total_tokens": 1240 }
}
```

Project example: `.cozyphi/hooks/cache-ratio/` logs `cache_ratio` and `cache_pct` to `.cozyphi/cache-ratio.jsonl` on each round.

### Failure policy (`fail_closed`)

| Value | When the script crashes, times out, or returns invalid JSON |
| --- | --- |
| `false` (default) | Ignore that hook (suitable for audit) |
| `true` | Deny (Pre / before_switch) or stop (Post) (suitable for security gates) |

In `permissions.mode: readonly`, only hooks with `fail_closed: true` run for the tool loop, so slow audit hooks do not stall exploratory tool use. Interactive sessions and `cozyphi run` run all loaded hooks. `session_start` / `session_shutdown` cannot set `fail_closed`, so they are skipped under the readonly FailClosedOnly view; use `session_before_switch` with `fail_closed` when a switch must be gated.

### Ordering and concurrency

- Matching **PreTool** hooks run **serially**. First deny wins; modify results chain onto `input`.
- Matching **PostTool** hooks run **in parallel** (except `async`, which is detached).
- **session_before_switch** runs **serially**. First deny wins.
- **session_start** / **session_shutdown** run **in parallel** (except `async`).
- Order across multiple hooks is **not** guaranteed. If order matters, put the logic in one hook.
- Because PostTool runs in parallel, do not rely on several hooks each rewriting `output` for the same tool call; put rewrite logic in one sync hook.

---

## Claude Code plugin hooks

A second, independent source of session hooks: the `SessionStart` and
`SessionEnd` hooks of Claude Code plugins enabled in Claude Code
(`~/.claude`) or listed under `plugins.paths` in `config.yaml`. See
`doc/plugins.md` for plugin discovery; this section covers only how their
hooks run. They reach cozyphi through `internal/hooks/claude.go`
(`ClaudeHook`), not through a `plugin.json` hook manifest, and only
`SessionStart`/`SessionEnd` are supported — every other event named in a
plugin's `hooks/hooks.json` is skipped with an "unsupported event" warning.

- **Events.** `SessionStart` maps to cozyphi's `session_start`, `SessionEnd`
  to `session_shutdown`. cozyphi's reason becomes Claude's `source`
  (`SessionStart`) or `reason` (`SessionEnd`):

  | cozyphi reason | `SessionStart.source` | `SessionEnd.reason` |
  | --- | --- | --- |
  | `startup` | `startup` | — |
  | `new` | `clear` | `clear` |
  | `resume` | `resume` | `resume` |
  | `compact` | `compact` | — |
  | `quit` | — | `prompt_input_exit` |
  | anything else | not run | `other` |

- **Matcher.** A hook entry's `matcher` in `hooks.json` applies to that
  `source`/`reason` value. Empty or `*` matches everything; otherwise it is
  anchored (`^(?:matcher)$`), so `startup|clear|compact` behaves exactly as
  it does in Claude Code — a bare substring never fires it, and an invalid
  expression skips that matcher group with a warning.
- **`bash -c` versus `args`.** A hook entry with `args` runs as `exec`
  without a shell; otherwise its `command` runs as `bash -c <command>`.
  Either way, `${CLAUDE_PLUGIN_ROOT}`, `${CLAUDE_PLUGIN_DATA}` and
  `${CLAUDE_PROJECT_DIR}` reach the process through the environment, not
  through text substitution of `command` — bash expands them itself, so a
  path holding a quote or `$()` cannot rewrite the command that runs. Only
  `args` entries are substituted textually, which keeps the same guarantee
  for the exec form. `DataDir` (`${CLAUDE_PLUGIN_DATA}`) is created before
  the first run; the working directory is the project root, not the
  directory the manifest lives in.
- **Timeout.** 30 s by default — Claude Code's own default is 600 s, but
  cozyphi runs `session_start` synchronously while the session opens, so it
  stays short — capped at the existing 60 s hook maximum from `hooks.json`'s
  `timeout` (seconds). `async: true` runs detached and its output is
  discarded.
- **Stdin.** One JSON object: `session_id`, `cwd`, `hook_event_name`, and
  `source` (`SessionStart`) or `reason` (`SessionEnd`). `transcript_path` is
  omitted: cozyphi's session file is not a Claude transcript, and a script
  that parses it as one would break.
- **Output parsing.** Exit 0: stdout that parses as JSON supplies context
  from `hookSpecificOutput.additionalContext`, top-level `additionalContext`,
  or `additional_context` (checked in that order), and a toast from
  `systemMessage`; any other non-empty stdout is context verbatim. Context is
  capped at 16 KiB, with the marker `"\n[plugin hook context truncated at 16
  KiB]"` appended on truncation — a separate, larger cap than the 4 KiB
  `context` cap of hook manifests, because a plugin bootstrap is a whole
  skill body, not a note.
- **Failures never block.** Any non-zero exit, a timeout, or the run being
  cancelled is non-blocking: it never denies or stops the session. The first
  line of stderr is redacted and written to the debug log
  (`COZYPHI_DEBUG=1`), and the failure also appears as a warning in
  `/hooks list`, via `hooks.Manager.Failures()`, until that hook next runs
  successfully.
- **Names.** A plugin hook entry is named `plugin:<Name>/<Event>#<n>` (`<n>`
  counts entries of that event within one plugin); `/hooks list` shows its
  source as `plugin:<Name>`. Plugin entries are additive: they are appended
  to what `hooks.Discover` finds and never shadow a hook manifest by name.
  `COZYPHI_HOOKS=off` and fail-closed-only mode (readonly permission mode)
  apply to plugin hooks exactly as they do to hook manifests.

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

| Field | PreTool | PostTool | Command | Session |
| --- | --- | --- | --- | --- |
| `session_id` | yes | yes | yes | yes |
| `cwd` | yes | yes | yes | yes |
| `hook_event` | `pre_tool` | `post_tool` | `command` | `session_*` / `post_turn` |
| `tool` | yes | yes | — | — |
| `tool_use_id` | yes | yes | — | — |
| `input` | yes | yes | — | — |
| `output` | — | tool stdout / result text when present | — | — |
| `error` | — | tool error text; empty on success | — | — |
| `command` | — | — | hook name | — |
| `args` | — | — | slash args after `/name` | — |
| `reason` | — | — | — | `startup` / `new` / `resume` / `compact` (`session_start` only) / `quit` |
| `previous_session_id` | — | — | — | start after switch |
| `target_session_id` | — | — | — | before_switch resume |
| `message_id` | — | — | — | post_turn assistant id |
| `usage` | — | — | — | latest completed assistant turn token counts |

### Environment

Sensitive parent environment keys are stripped before spawn (substring match, case-insensitive), including patterns such as `API_KEY`, `SECRET`, `TOKEN`, `PASSWORD`, `COZYPHI_API_KEY`, and common cloud credential names.

Injected variables:

| Variable | Value |
| --- | --- |
| `COZYPHI_HOOK_EVENT` | `pre_tool`, `post_tool`, `command`, or `session_*` |
| `COZYPHI_SESSION_ID` | Session id |
| `COZYPHI_CWD` | Workspace cwd |
| `COZYPHI_PROJECT_DIR` | Same as cwd for command hooks |

---

## Operations

| Action | How |
| --- | --- |
| Disable all hooks | `COZYPHI_HOOKS=off` |
| Inspect load warnings | `COZYPHI_DEBUG=1` |
| List / reload in TUI | `Ctrl+K` → **hooks → list** / **hooks → reload** |
| Override a user hook | Declare the same hook `name` under `<cwd>/.cozyphi/hooks/<name>/plugin.json` |
| Disable Claude Code plugin discovery | `COZYPHI_PLUGINS=off` (leaves hook manifests running; see `doc/plugins.md`) |

Configuration for hooks is **not** stored in `~/.cozyphi/config.yaml` or managed via `cozyphi config`.

---

## Limitations

The following are intentionally out of scope:

- Long-lived hook host processes or bidirectional RPC
- File-watch based hot reload (use palette reload or restart)
- Registering new tools from hooks (use `tooldef.Tool`)
- Mixing hook definitions into the main YAML config

---

## Related code

| Path | Role |
| --- | --- |
| `internal/hooks/` | Types, Manager, discovery (`plugin.json`), CommandHook, Load |
| `internal/hooks/claude.go` | `ClaudeHook`: parses a plugin's `hooks.json`, runs `SessionStart`/`SessionEnd` |
| `internal/plugin/` | Discovers Claude Code plugins into skill sources and hook files (see `doc/plugins.md`) |
| `internal/agent/executor.go` | Pre → Gate → Run → Post |
| `internal/project` | `HooksDir()`, directory bootstrap |
| `internal/tui` | Engine wiring; list / reload; `HookCommands` registers slash commands |
