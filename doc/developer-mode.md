# Developer mode

Developer mode gives the model one extra tool, `harness`, and that tool reads
cozyphi's own configuration: what this process is running with, where each
value came from, and what it would take to change it. It is the answer to
"why did it do that" asked from inside the session that did it, instead of
from a config file that may not be the one in force.

It is read-only in the strong sense — there is no write action, no reload, no
reset, and observing runs no command, opens no connection and touches no file.

| Audience | This document |
| --- | --- |
| Users | How to switch it on, what the model can then see, and what it still cannot |
| Contributors | The three layers, the five states, the bounds, and what is deliberately withheld |

---

## Switching it on

```sh
cozyphi --developer-mode                 # TUI
cozyphi tui --developer-mode             # the same thing, spelled in full
cozyphi --developer-mode --resume ID     # one process; every session it opens observes
cozyphi run -p "..." --developer-mode    # one headless run
```

The flag is the only grant, and it is deliberate:

- **No config key, no environment variable.** Nothing in `config.yaml` and no
  `COZYPHI_*` variable switches it on. A machine cannot end up in developer
  mode because a file was checked in.
- **No slash command, no runtime toggle.** The grant is fixed before the first
  session is built; a process that started without it cannot acquire it.
- **Resuming restores nothing.** A session recorded in a developer-mode
  process, reopened by an ordinary one, is an ordinary session.
- **Sub-agents never inherit it.** A child spawned by a developer-mode session
  carries no `harness` tool.

Without the flag the tool is not registered at all. A call that reaches it
anyway — through a stale tool list, say — is refused with the one thing worth
saying:

```text
harness: this session was not started with --developer-mode, so it observes
nothing. There is no way to enable it from here; the user must restart
cozyphi with the flag
```

Developer mode is **not a sandbox** and asks for **no special consent**. It
adds a tool; that tool sits behind the ordinary permission gate exactly like
every other one, and switching it on relaxes nothing else about the session.

## The tool

One tool, `harness`, with three actions.

| Action | Arguments | Returns |
| --- | --- | --- |
| `catalog` | none | Every category, whether it can be observed, and the field keys it declares |
| `snapshot` | `category?` | Without a category, the overview: every category reduced to what is acting now. With one, that category alone, in full |
| `explain` | `category`, `key` | One field with everything known about it |

The flow is `catalog → snapshot → explain`: find the category, see its shape,
then ask about the one field that matters. A wrong shape is refused rather
than guessed at — `catalog` takes no category, `snapshot` takes no key, and
`explain` needs both.

### The categories

Eleven, in this order, and the order is fixed so two answers can be compared.

| Category | What it observes |
| --- | --- |
| `runtime` | Version and build, the process's mode, the workspace root, this session's id, whether developer mode is on |
| `model` | The model selected, loaded and answering now; effort, window, output cap, variants; the provider catalog and the opencode import |
| `context` | The window this session budgets against, what occupies it, what compaction will do about it, and what the system prompt was assembled from |
| `permissions` | The boundary this session judges tool calls with: mode, bypass, bash default, containment, and the shape of its rules |
| `plan` | Where the durable plan stands, what policy the gate compiles from, and what the step in progress may do |
| `tools` | Which tools this session carries, why a known one is missing, and what stands between the model and calling it |
| `integrations` | MCP servers, language servers and hooks: which are configured, which are reachable, and what the last exchange observed |
| `agents` | Whether sub-agents are on, which roles and models a spawn may name, the nesting and concurrency ceilings, and what is out now |
| `storage` | Where this session's state is kept — sessions, memory, the task registry, usage history — and how much of it there is |
| `ui` | The surface this session runs behind: palette, key dialect, notification delivery, speech input |
| `diagnostics` | What this session has watching the world for it, and what the process observes about itself: watches, debug logging, telemetry, profiling, and the harness's own limits |

Every category is listed in the catalog whether or not it can answer, so a
gap in the harness is visible rather than absent. Each entry carries an
availability:

| Availability | Meaning |
| --- | --- |
| `available` | Wired, and it can observe now. All eleven are, in a fully assembled session |
| `unavailable` | A collector exists but could not answer this time; `reason` says why |
| `not_applicable` | The category cannot exist in this process shape |
| `not_implemented` | No collector is wired for this category. No category is in this state today |

## What one field carries

Three layers, because they disagree more often than they look like they
should, and the disagreement is usually the answer:

| Layer | Question it answers |
| --- | --- |
| `configured` | What a source asked for |
| `loaded` | What the owner took in |
| `effective` | What is acting right now |

Each layer names its own source, and the source kinds are also an override
order: `default` is replaced by `config_file`, that by `env`, that by a
`session` choice, and all of them — for as long as a step runs — by a `plan`
pin. `cli_flag`, `computed`, `build` and `unknown` are the four that sit
outside the ladder.

An empty-looking answer is never ambiguous. Five states are told apart, and
`false` and `0` are real values that always appear:

| State | Meaning |
| --- | --- |
| `present` | The value below is the real one |
| `unset` | Nothing set this layer; the value is the zero a caller would get |
| `redacted` | A real value existed and had to be masked |
| `unavailable` | The owner could not answer right now |
| `not_applicable` | This layer does not exist for this field — a build constant has no configured layer |

A field also says what it would take for a change to it to land — cheapest
first — and how far its value reaches — widest first:

| `apply` | What a change to this field waits for |
| --- | --- |
| `immediate` | Nothing; it acts as soon as it is made |
| `next_turn` | The next round of the conversation |
| `reload` | An owner to re-read the configuration — editing the file on disk is not enough on its own |
| `new_session` | A new session, in the same process |
| `restart` | A new process |

| `scope` | What the value covers |
| --- | --- |
| `process` | Everything this cozyphi process does |
| `session` | This session, and not its siblings in the same process |
| `workspace` | The directory this session was opened in |
| `turn` | The round in flight |
| `step` | The plan step in progress |

And it says how close to now the answer is. `observed_at` is when the question
was answered; `freshness` is whether the owner was read to answer it —
`live` when it was, `published` when the owner handed its account over earlier
and this is that account, `unknown` when nobody can say. `revision` changes
when the underlying state does, so two answers that look alike can be told
apart.

## Reading an answer

The examples below come from a fixture session: one model `kestrel` in
`config.yaml`, `compact_threshold: 150000`, `notifications.mode: unfocused`,
nothing rendered yet.

`catalog` names what exists — one entry per category, trimmed here to two
of the eleven and with the long `reason` text cut:

```jsonc
{"categories": [
  {"category": "runtime", "availability": "available", "reason": "",
   "keys": ["version", "build.commit", "build.date", "mode",
            "developer_mode", "workspace.root", "session.id"]},
  {"category": "ui", "availability": "available",
   "reason": "the surface this session runs behind: …",
   "keys": ["surface.kind", "theme.name", "theme.builtin", "keymap.mode",
            "keybinds.commands", "keybinds.overrides", "notify.mode",
            "notify.sound", "notify.delivery", "notify.focus", "voice.state",
            "voice.backend", "voice.capture", "voice.model",
            "voice.credential"]}
]}
```

`explain` is one field with everything. This is a whole answer, exactly as
the tool returns it:

```json
{
  "category": "runtime",
  "availability": "available",
  "reason": "",
  "observed_at": "2026-09-07T05:17:08.861876+03:00",
  "field": {
    "key": "developer_mode",
    "configured": {
      "state": "present",
      "value": {
        "kind": "bool",
        "string": "",
        "int": 0,
        "bool": true,
        "list": []
      },
      "source": {
        "kind": "cli_flag",
        "ref": "--developer-mode"
      }
    },
    "loaded": {
      "state": "present",
      "value": {
        "kind": "bool",
        "string": "",
        "int": 0,
        "bool": true,
        "list": []
      },
      "source": {
        "kind": "cli_flag",
        "ref": "--developer-mode"
      }
    },
    "effective": {
      "state": "present",
      "value": {
        "kind": "bool",
        "string": "",
        "int": 0,
        "bool": true,
        "list": []
      },
      "source": {
        "kind": "cli_flag",
        "ref": "--developer-mode"
      }
    },
    "apply": "restart",
    "scope": "process",
    "observed_at": "2026-09-07T05:17:08.861876+03:00",
    "freshness": "live",
    "revision": ""
  }
}
```

Every member of a value is serialized every time — that is why `int` and
`bool` are both there on a `kind: "bool"` value. It is what keeps a `false`
from reading as an absence. The examples below are trimmed to the layers being
discussed.

Layers that disagree are the point. Nothing has rendered in this session, so
the mode somebody configured is answered and the delivery it would produce is
not:

```jsonc
{"key": "notify.mode",
 "configured": {"state": "present", "value": {"kind": "string", "string": "unfocused"},
   "source": {"kind": "config_file", "ref": "the notifications.mode the configuration was loaded with"}},
 "effective":  {"state": "unavailable", "value": {"kind": "none"},
   "source": {"kind": "unknown", "ref": "no surface has published its state to this session yet, so what one would be doing is not something this view may reach for on its own"}},
 "freshness": "published", "apply": "immediate", "scope": "session"}
```

A field with no configured layer says so rather than showing an empty one —
`notify.delivery` is the mode, the terminal's focus and the sender's health
added up, and nothing configures it on its own:

```jsonc
{"key": "notify.delivery",
 "configured": {"state": "not_applicable", "value": {"kind": "none"},
   "source": {"kind": "computed",
     "ref": "nothing configures the outcome on its own: it is the mode, the terminal's focus and the sender's health added up"}}}
```

A missing tool is explained rather than omitted. The fixture workspace has no
task registry, so the tool that works one is not there — a fact about the
workspace, not a denial:

```jsonc
{"key": "tool.task",
 "effective": {"state": "present", "value": {"kind": "string", "string": "unavailable"},
   "source": {"kind": "session", "ref": "no task registry was discovered for this workspace"}},
 "revision": "1ebe016cec56fd03"}
```

Asking for a key a category does not declare gets the keys it does:

```text
harness: diag: category "context" has no field "context.window"; it declares:
window, window.ceiling, window.override, usage.tokens, usage.token_source, …
```

## Bounds

Answers are size- and time-bounded, and the bounds are themselves observable
under `diagnostics` → `harness.limits`:

```text
fields_per_category=64  value_bytes=512  list_items=32
total_bytes=16384       category_time=2s  answer_time=5s
```

The byte budget is split across the categories still to be read, so a
category's place in the catalog decides nothing about how much it may say.
When an answer does not fit, it says so and there is **no paging** — the
caller narrows instead, first to a category, then to a key. In the fixture
above the overview is exactly that case:

```jsonc
{"mode": "overview", "truncated": true, "partial": false,
 "note": "output hit a size limit and fields were dropped; narrow it with category, then with key"}
```

`truncated` means the size budget dropped something; `partial` means a
category could not be observed, and that category's own `reason` says why. A
category whose owner was busy, or that the answer's time ran out before
reaching, is still listed — with a reason that asks for it on its own. A
category that vanished from an answer would read as a category that does not
exist.

## What it will not show

The rule is that the view reports *shape*, not *contents*.

- **Secrets are presence and kind, never value.** The `model` category's
  `credential` field is a boolean, and no api key, token, hash or suffix
  appears in any layer. If a value that reached the boundary still looks like
  a secret, it is masked and the layer reports `redacted` — a backstop, not
  the normal path.
- **Whole classes of value have no field at all**: raw config structs,
  environment variables, prompts, transcripts, memory contents, logs, hook and
  watch command lines, MCP server arguments and environments, and server tool
  schemas.
- **Rules are counted and attributed, never quoted.** No bash pattern,
  sensitive path prefix, MCP allow entry or memory path leaves the process.
- **Provider and backend endpoints are withheld**, because a URL can carry a
  token in its path or its query. Who the provider is and whether a
  credential exists are reported instead, as `model` → `provider` and
  `model` → `credential`.
- **Paths are anchored and sanitized** — the home directory collapses to `~`,
  control characters are dropped — so a value here can read shorter than the
  original.

Some settings are withheld permanently because they are not about the harness
at all: TUI panel geometry, the speech capture command line and audio device
name, the machine's `PATH`, and the environment variables that only point the
read-only opencode import at a different file.

Others simply have no field yet, and the honest answer is that they are
missing rather than that they are unavailable: the whole `web` tool policy
(egress, hosts, schemes, search provider, download limits), part of the voice
settings (language, hints, glossary, timeouts), `COZYPHI_MCP_LOG_DIR` and
`COZYPHI_PLAN_GATE_LOG_DIR`, and the headless run's `--jsonl`,
`--max-rounds` and `--timeout`. Each is tracked; none of them is reported as
anything else in the meantime.

## What reading it does

Nothing. Producing an answer calls no tool, preflights no call, asks for no
approval and moves no plan step; it starts no MCP or language server, runs no
hook or watch command, sends no notification, opens no microphone, renders no
frame, reloads no configuration, re-reads no preferences file, compacts
nothing, loads no memory and writes nothing to disk. Collectors read their
owner through accessors and return; the registry sanitizes, bounds, detaches
and timestamps.

The one thing a question can leave behind is a record, and only when the debug
log is already on (`COZYPHI_DEBUG=1`, see [Hooks](hooks.md) for the log's other
uses). The record is an allowlist by construction — what was asked, how far it
reached, how long it took, and how it ended:

```text
harness snapshot category=ui mode=detail result=answered partial=false truncated=false categories=1 fields=15 elapsed=1ms
```

There is no member in it for a value, a source, a reason or an error message
to travel in. `result` is one of `answered`, `refused` (turned down before
anything was observed), `failed` (an owner could not be read) or `canceled`
(the caller stopped waiting — an aborted turn, usually, and not an error).
