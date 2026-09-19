# Protected web channel inventory

Evidence inventory for [web-research-01-channel-inventory](../obsidian-tasks/web-research-01-channel-inventory.md),
answering [protected-web-research.md](protected-web-research.md) D7 (whole-harness
protection), D15 (platforms and limited operation), D16 (relationship to existing
security contracts) and traceability rows Q13/Q26/Q39, per the delivery rules in
[protected-web-research-tickets.md](protected-web-research-tickets.md).

- **Revision:** worktree `docs/web-research-01-channel-inventory` at commit
  `7162d11f` ("chore(tasks): close web ticket publishing, start channel
  inventory"), one task-registry commit on top of base `71b2277d`. All file:line
  references below are at `7162d11f`; nothing outside this tree was assessed.
- **Landed vs unmerged:** only code at this commit counts. Sibling worktrees
  (e.g. `bug/bound-mcp-framing-and-output`, `feature/bash-failure-evidence`,
  visible in `git worktree list`) are unmerged experiments and contribute no
  evidence here.
- **Design is not proof:** the linked design task is `todo` and unstarted; no
  process-isolation implementation exists anywhere in this tree.

## Classification vocabulary

- **ready** — enforcement exists in landed code and a public-boundary test or
  reproducible check exists that a reviewer can run today.
- **disabled** — the channel is off by default or refuses by configuration, with
  landed evidence of the refusal.
- **unproved** — the channel is enabled but no landed control, or no landed
  *evidence*, establishes the D7 property for it. "Unproved" includes channels
  whose only control is a command-name check, which D7 explicitly rejects
  ([protected-web-research.md](protected-web-research.md) D7: "command-name deny
  lists cannot establish this property").

## Host coordination today

There is one coordination point per process: the permission gate chain, assembled
as `TaintGate{Inner: BypassGate{Inner: StaticGate}}`.

- `TaintGate` wraps the whole boundary including the bypass
  (`internal/permission/taint.go:40-58`); the engine installs it at
  `internal/agent/engine.go:800`; the TUI controller installs `BypassGate`
  ("Allow All for This Session") at `internal/tui/controller/controller.go:555-564`.
- The executor enforces the order Pre-hooks → plan gate → permission gate → Run →
  Post-hooks for every tool call (`internal/agent/executor.go:369-413`, dispatch
  and post-hooks at `internal/agent/executor.go:485-536`).
- Cross-process coordination does not exist: sessions hold a per-process
  ownership lock (`internal/session/ownership.go`, platform splits
  `ownership_unix.go` / `ownership_windows.go`) and the job manager is a
  per-process object constructed in `cmd/run.go:355-373`. There is no
  cross-session block/release mechanism of the kind D9 requires.
- **Unresolved owner H** (per
  [protected-web-research-tickets.md](protected-web-research-tickets.md),
  "External prerequisites"): no existing task owns cross-process web
  block/release coordination. The closest landed seams are `internal/permission`
  (per-process gate), `internal/session/ownership.go` (single-process session
  lock) and `internal/job/manager.go` (per-process job manager). None is an H
  implementation; the owner remains unidentified — recorded as a blocker below.

## Channel inventory

Each entry records: owner, applicable shared security task, current enforcement
with file:line evidence, a reproducible public-boundary check (or "none
exists"), residual gap, and classification.

### C1. Ordinary file tools — read / grep / find / ls

- **Owner:** `internal/tools/readtool`, `internal/tools/greptool`,
  `internal/tools/findtool`, `internal/tools/lstool`; gate
  `internal/permission/gate.go:123-124` → `checkPaths`
  (`internal/permission/gate.go:352-406`).
- **Shared security task:** none directly; provenance depends on
  [security-durable-provenance](../obsidian-tasks/security-durable-provenance.md)
  (status `blocked`).
- **Enforcement today:** physical-target symlink resolution with fail-closed
  errors (`internal/permission/gate.go:367-369`); sensitive-path deny list
  covering `~/.ssh`, `~/.cozyphi/config.yaml`, `~/.aws/credentials`, `~/.gnupg`,
  `/etc/shadow` (`internal/permission/rules.go:13-45`). **Reads are not
  workspace-confined:** `WorkspaceOnlyReads: false` in the default policy
  (`internal/permission/policy.go:238`), and no `workspace_only_reads` YAML key
  exists to change it (grep for `workspace_only_reads` / `WorkspaceOnlyReads`
  in `internal/project` returns nothing).
- **Public-boundary check:** `go test ./internal/permission/` (gate, symlink,
  platform rule tests; not run for this docs task). None exists that proves web
  material is unreachable through this channel.
- **Residual gap:** the web cache (`~/.cozyphi/web`,
  `internal/project/project.go:73`, filled as the default at
  `internal/project/config.go:353-354`), job artifacts (`~/.cozyphi/jobs`,
  `internal/project/project.go:116-117`) and session transcripts
  (`~/.cozyphi/session/<encoded-cwd>/`, `internal/project/session_dir.go:55-61`)
  are all outside the workspace and absent from the sensitive deny list, so the
  `read` tool can return raw cached page text and job transcripts to the main
  model — a direct D7 raw-cache/transcript bypass. Whether the cozy-tools cache
  is even plaintext is **unknown**: fetch/cache implementation lives in the
  external module `github.com/alvnukov/cozy-tools v0.2.0` (`go.mod:10`), not in
  this repository.
- **Classification:** unproved.

### C2. Ordinary file tools — write / edit

- **Owner:** `internal/tools/writetool`, `internal/tools/editledger`; gate
  `internal/permission/gate.go:121-122` → `checkWrite`
  (`internal/permission/gate.go:332-334`).
- **Shared security task:** none.
- **Enforcement today:** workspace-only writes by default
  (`internal/permission/policy.go:231`); writes to git control files ask for
  consent (`internal/permission/gate.go:397-403`,
  `internal/permission/rules.go:47-88`); memory directory exemption from the
  workspace rule (`internal/permission/gate.go:380-391`).
- **Public-boundary check:** `go test ./internal/permission/` (gate,
  `gate_symlink_test.go`). None exists for protected-web scenarios.
- **Residual gap:** a workspace write can plant a project hook
  (`<repo>/.cozyphi/hooks`, `internal/project/project.go:121-124` — see C5), a
  skill file, or task-registry content that later steers the model; the gate
  vets paths, not future-execution payloads. Nothing binds a write to the
  provenance of the content being written.
- **Classification:** unproved (generic filesystem gate ready; protected-web
  property unproved).

### C3. Shell and Python execution (bash tool)

- **Owner:** `internal/tools/bashtool` (definition `bash.go:30-34`; execution
  `shell_exec.go:27` via `bash -c`; shell resolution incl. Windows Git Bash
  `shellconfig.go:3-16`); gate `checkBash`
  (`internal/permission/gate.go:303-330`).
- **Shared security tasks:**
  [security-process-isolation-design](../obsidian-tasks/security-process-isolation-design.md)
  (status `todo`, **design only, unstarted**),
  [refactor-external-binary-runner](../obsidian-tasks/refactor-external-binary-runner.md)
  (status `todo`, managed-subprocess seam not landed).
- **Enforcement today:** command-name regex deny list
  (`internal/permission/defaults.go:26-39` — sudo, `rm -rf`, `curl|sh`, etc.),
  allowlist restricted to single simple commands with shell-metacharacter
  parsing (`internal/permission/rules.go:217-283`), default Ask
  (`internal/permission/policy.go:234`). After web taint, bash is re-asked
  within the turn (`internal/permission/taint.go:63-66`). No process, filesystem,
  environment or network isolation: the child inherits the full environment
  (`internal/tools/bashtool/command.go:19`, `os.Environ()`), and any approved —
  or session-bypassed (`internal/permission/bypass.go:19-30`) — command may
  fetch arbitrary URLs (`curl`, `wget`, `python3 -c ...`; Python has no
  separate tool, it is this channel) and print the result into the transcript.
  Output is display-bounded at 1000 lines / 50 KiB
  (`internal/tools/bashtool/bash_output.go:13-16`) with the full log in
  `$TMPDIR/cozyphi-bash-*.log` (`internal/tools/bashtool/bash_output.go:125`),
  itself readable via C1. Background bash exists (`background.go:47-72`).
- **Public-boundary check:** `go test ./internal/permission/` and
  `go test ./internal/tools/bashtool/` cover gate parsing and output bounding.
  **None exists** for network/filesystem/environment isolation — no isolation
  mechanism exists to test (D15 requires disabling this channel in a restricted
  mode rather than claiming safety).
- **Residual gap:** turn taint resets on every new turn
  (`internal/agent/web.go:114-124`), so a new user message launders yesterday's
  web-tainted session into a clean bash approval — D7 explicitly rejects this
  ("A new user message does not cleanse old web content"). One approval, one
  allowlist entry, or allow-all buys arbitrary egress.
- **Classification:** unproved (unmediated; candidate for "disabled" in a
  restricted mode per D15, but no restricted mode exists in code).

### C4. MCP servers (mcp_list / mcp_inspect / mcp_call)

- **Owner:** `internal/tools/mcptool/mcp.go` (three meta-tools, `:16-25`);
  transports `internal/mcp/stdio.go`, `internal/mcp/http.go`; gate
  `internal/permission/gate.go:175-200`.
- **Shared security tasks:**
  [sandbox-mcp-stdio-environment](../obsidian-tasks/sandbox-mcp-stdio-environment.md)
  (status `todo`; its own body cites `internal/mcp/stdio.go` ambient
  `os.Environ()` inheritance — confirmed at `internal/mcp/stdio.go:148`),
  [refactor-external-binary-runner](../obsidian-tasks/refactor-external-binary-runner.md)
  (status `todo`).
- **Enforcement today:** schemas stay off-context — `mcp_list`/`mcp_inspect`
  return compact text and are allowed without asking
  (`internal/permission/gate.go:175-178`); `mcp_call` asks by default and can be
  pre-approved per server/tool via `permissions.mcp.allow`
  (`internal/permission/gate.go:181-200`). After web taint, `mcp_call` is
  re-asked within the turn (`internal/permission/taint.go:65`). Call results are
  bounded to 32 000 bytes (`internal/tools/mcptool/mcp.go:171`); HTTP transport
  bodies capped at 8 MiB (`internal/mcp/http.go:21,85`).
- **Public-boundary check:** `go test ./internal/mcp/ ./internal/tools/mcptool/`
  (bounds, desync, workspace tests). **None exists** for isolation of server
  processes or for framing of server output.
- **Residual gap:** MCP server output reaches the model as ordinary tool text
  with no untrusted frame; a configured server can itself fetch web content and
  return it, bypassing the web quarantine entirely. Stdio servers inherit every
  ambient credential (`internal/mcp/stdio.go:148`) and run unsandboxed; HTTP
  servers receive configured headers (`internal/mcp/http.go:44-45`). One
  `mcp.allow` entry pre-approves a server permanently.
- **Classification:** unproved.

### C5. Hooks (pre / post / command / session)

- **Owner:** `internal/hooks` (discovery `discover.go:56-63`, command execution
  `command.go:389`, env sanitization `sanitize.go:17-60`); wired by the executor
  (`internal/agent/executor.go:369-395`, `:495-507`); discovery roots are user
  `~/.cozyphi/hooks` and **project** `<repo>/.cozyphi/hooks`
  (`internal/tui/controller/controller.go:774`,
  `internal/project/project.go:121-124`).
- **Shared security task:**
  [security-process-isolation-design](../obsidian-tasks/security-process-isolation-design.md)
  (status `todo`; its acceptance criteria name prehook launch and secret-safe
  diagnostics — design only).
- **Enforcement today:** hooks run as direct `exec` without a shell
  (`internal/hooks/command.go:21-22,389`); output capped at 1 MiB
  (`internal/hooks/command.go:16`); environment stripped of keys matching
  credential substrings (`internal/hooks/sanitize.go:18-34`); deny/stop contract
  via exit code 2 (`internal/hooks/command.go:19`,
  `internal/agent/executor.go:510-518`). `COZYPHI_HOOKS=off` disables discovery
  (`internal/hooks/discover.go:48-53`).
- **Public-boundary check:** `go test ./internal/hooks/ ./internal/agent/`
  (`hooks_observe_test.go`, `hookstop_test.go`). **None exists** for hook-process
  isolation or for trust of project-shipped hooks.
- **Residual gap:** hook results are an unframed text channel into the model:
  `PreResult.Context` is a "model-facing note" and `PreResult.Input` rewrites
  tool arguments (`internal/hooks/types.go:107-112`,
  `internal/agent/executor.go:378-395`); `PostResult.Output` **replaces** the
  tool result the model sees (`internal/hooks/types.go:114-120`,
  `internal/agent/executor.go:495-507`). Project hooks are discovered and run
  with no consent step found in `internal/hooks/load.go`/`manager.go` (grep for
  trust/approval: no match), so any workspace write (C2) that lands
  `.cozyphi/hooks/**` becomes code execution whose output reaches the model.
- **Classification:** unproved.

### C6. Watches (background command events)

- **Owner:** `internal/tools/watchtool/watch.go`; runtime `internal/watch`;
  model delivery `internal/agent/watch.go:25-46`; gate `checkWatch`
  (`internal/permission/gate.go:283-301`).
- **Shared security task:**
  [security-process-isolation-design](../obsidian-tasks/security-process-isolation-design.md)
  (status `todo`; its acceptance criteria name background watches explicitly).
- **Enforcement today:** start is judged by the bash deny list and bash default,
  never the bash allowlist (`internal/permission/gate.go:283-301`); watch output
  reaches the model only inside a `<system-reminder>` that names the source and
  denies instruction status (`internal/agent/watch.go:25-46`); bounds on event
  size, events per delivery, flood, live count and interval
  (`internal/tools/watchtool/watch.go:23-30`); sub-agents and headless runs get
  no manager (`internal/tools/watchtool/watch.go:106-110`). Tainted turns
  re-ask for start/stop (`internal/permission/taint.go:67-68`).
- **Public-boundary check:** `go test ./internal/permission/ -run Watch` and
  `internal/agent/engine_watch_test.go`. **None exists** for isolating the
  watched command's filesystem/network access.
- **Residual gap:** the watch command itself is an unsandboxed shell command
  (`internal/watch/watch.go:124-127` runs it through the bash tool's shell); one
  approval buys unbounded repeated execution, and a command such as a polling
  `curl` turns watch events into a standing web-ingress channel. Framing is a
  prompt-level mitigation, not isolation; turn taint resets per turn (C3).
- **Classification:** unproved.

### C7. Sub-agents and delegated jobs (agent_spawn / agent_wait / job results)

- **Owner:** `internal/tools/agenttool/agent.go` (`:66-93`);
  `internal/job` (store files `store.go:14-18`: `meta.json`, `events.jsonl`,
  `result.md` under `~/.cozyphi/jobs`, `internal/project/project.go:116-117`);
  role profiles `internal/agent/child_spec.go:26-62`; outcome delivery
  `internal/agent/outcomes.go` (`AcceptOutcome`).
- **Shared security task:**
  [security-durable-provenance](../obsidian-tasks/security-durable-provenance.md)
  (status `blocked`) for provenance through child outcomes; no task owns
  child-process isolation (process-isolation design is `todo`).
- **Enforcement today:** children cannot spawn agents (`agent.go:39`,
  `child_spec.go` tool sets carry no `agent_*`); explore/review are read-only,
  worker runs headless-strict (`child_spec.go:40-61`); the parent receives only
  the final summary, delivered as framed untrusted data with JSON-escaped
  wrapper (`internal/agent/outcomes.go:20-33`); spawn confinement validated
  against the parent workspace at `job.Spawn`
  (`internal/permission/gate.go:125-128`); summary capped at 12 000 bytes
  (`internal/tools/agenttool/agent.go:20`); outcome delivery is
  identity-checked and acknowledged against owner/session
  (`internal/agent/outcomes.go`). Spawnable roles may carry the web tool; the
  quarantine-reader role may not (`internal/agent/child_spec.go:87-101`).
- **Public-boundary check:** `go test ./internal/agent/ -run 'Spawn|Outcome'`
  and `go test ./internal/job/ ./internal/tools/agenttool/`. **None exists**
  for provenance persistence of web-derived child summaries across resume/fork.
- **Residual gap:** the child summary is model-written text that may derive
  from web fetches the child performed (web allowed for all spawnable roles);
  it enters the parent framed but with no durable provenance record — D7
  requires lineage through "child outcomes", and
  [security-durable-provenance](../obsidian-tasks/security-durable-provenance.md)
  is `blocked`. Job transcripts and `result.md` on disk are readable through C1
  (outside workspace, not on the deny list), so unchecked child context
  reachable via `read` bypasses the framed-summary boundary.
- **Classification:** unproved.

### C8. Model requests and responses (internal/llm, internal/agent, web tool)

- **Owner:** providers `internal/llm/anthropic`, `internal/llm/openai`,
  `internal/llm/responses`; engine `internal/agent/engine.go`; web ingress
  `internal/agent/web.go` + `internal/tools/webtool`.
- **Shared security tasks:**
  [security-model-egress](../obsidian-tasks/security-model-egress.md) (status
  `blocked`) for effective-request mediation;
  [security-durable-provenance](../obsidian-tasks/security-durable-provenance.md)
  (status `blocked`).
- **Enforcement today (web ingress — the strongest landed channel):**
  - web tool is off unless a `web:` config section opts in
    (`internal/project/config_web.go:62-73`); the gate independently denies all
    web actions when disabled (`internal/permission/gate.go:222-225`).
  - `fetch` returns metadata only, never the body
    (`internal/tools/webtool/web.go:152-161`); `read`/`find` default to the
    tool-less quarantine reader (`internal/agent/web.go:199-239`), whose entire
    tool list is decoys (`internal/tools/webtool/reader.go:17-79`); any tool
    call aborts the document and flags it (`internal/tools/webtool/web.go:22`,
    `:313-345`).
  - `raw:true` asks the user every time (`internal/permission/gate.go:226-228`).
  - untrusted frame with JSON-escaped wrapper and fixed preamble
    (`internal/tools/webtool/frame.go:9-37`).
  - egress length cap (2048) and env-secret mask before any request
    (`internal/tools/webtool/egress.go:15,42-57,78-94`).
  - turn taint re-asks mutating/egress/spawn actions for the rest of the turn,
    wrapping even the allow-all bypass (`internal/permission/taint.go:24-58`,
    `:63-78`).
- **Enforcement today (model egress):** none beyond provider client code.
  `internal/llm/*` sends the session context to the configured provider; there
  is no per-recipient grant, no recipient validation, no mediation layer —
  exactly what [security-model-egress](../obsidian-tasks/security-model-egress.md)
  (`blocked`) is meant to add.
- **Public-boundary check:** `go test ./internal/tools/webtool/
  ./internal/permission/ ./internal/agent/` (`web_test.go`, `egress_test.go`,
  `frame_test.go`, `web_taint_test.go`, `web_role_test.go`,
  `permission/web_test.go`, `taint_test.go`, `bypass_test.go`). **None exists**
  for durable cross-turn provenance or model-egress mediation.
- **Residual gap:** taint is per-turn by design (`internal/agent/web.go:114-124`)
  and quarantine mode `off` is a supported user configuration that hands bounded
  raw fragments straight to the session (`internal/project/config_web.go:14-21`)
  — outside the protected contract by the user's own choice. D7's durable
  provenance (surviving turns, resume, fork, compaction) has no implementation.
- **Classification:** web read/find quarantine path — **ready** for its
  documented within-turn scope (landed tests named above); `raw:true` — ready as
  an always-asked gate; quarantine `off` mode — **disabled** protection by user
  configuration; durable provenance and model egress — unproved.

### C9. Caches, transcripts, session persistence, compaction, memory

- **Owner:** web cache — external `github.com/alvnukov/cozy-tools v0.2.0`
  (`go.mod:10`), directory `~/.cozyphi/web` (`internal/project/project.go:73`,
  default wired at `internal/project/config.go:353-354`); sessions —
  `internal/session` (`~/.cozyphi/session/<encoded-cwd>/`,
  `internal/project/session_dir.go:55-61`); compaction —
  `internal/agent/engine_compaction.go`, `internal/session/compaction`;
  memory — `internal/memory` (`~/.claude/projects/<encoded>/memory/`,
  described in `internal/memory/doc.go` and `internal/permission/policy.go:216-225`).
- **Shared security tasks:**
  [security-storage-design](../obsidian-tasks/security-storage-design.md)
  (status `todo`, **design only**) and
  [security-encrypted-sessions](../obsidian-tasks/security-encrypted-sessions.md)
  (status `blocked`) for protected persistence;
  [security-durable-provenance](../obsidian-tasks/security-durable-provenance.md)
  (status `blocked`).
- **Enforcement today:** session replay strips balanced `<system-reminder>`
  blocks (including framed web content and watch reminders) from history
  (`internal/memory/recall.go:188-191`, usage noted at
  `internal/agent/engine.go:1356` and `internal/tools/webtool/frame.go:24-27`);
  the memory tool reads/prunes and never writes facts
  (`internal/permission/gate.go:261-270` and AGENTS.md memory invariant); memory
  facts are written with the ordinary write tool into the memory directory,
  which is exempted from workspace-only rules but not from the sensitive deny
  (`internal/permission/gate.go:376-391`, `:430-435`).
- **Public-boundary check:** `go test ./internal/session/ ./internal/memory/`
  (load, torn-tail, recall tests). **None exists** for provenance of compaction
  summaries or memory facts derived from web content.
- **Residual gap:** compaction summaries are model-written text re-entering
  context with no provenance marker (`internal/agent/engine_compaction.go:34-58`,
  summary preparation `internal/session/compaction/compact.go`) — D7's
  "conservatively inherit input dependencies rather than laundering a summary
  into clean context" is unimplemented. No encryption at rest anywhere (storage
  design `todo`, encrypted sessions `blocked`). Web cache storage format:
  **unknown** (external module). All of these artifacts are readable by the
  main model through C1.
- **Classification:** unproved.

### C10. Diagnostics and logs that reach the model

- **Owner:** `internal/tools/harnesstool/harness.go` (registered only under
  `--developer-mode`, `:5,:33-36,:93`); sanitizing boundary
  `internal/diag/sanitize.go` (secret-shape masking, home collapse, control
  characters, byte caps); LSP diagnostics via `internal/tools/lsptool`
  (`lsp.go:36`, render `render.go:114-117`); file logging `internal/debuglog`
  (file-only, not model-facing); desktop notifications `internal/notify`
  (`sender_darwin.go:1` osascript with argv-passed text — egress to the OS, not
  a model-ingress route).
- **Shared security task:** none specific;
  [security-observe-guard](../obsidian-tasks/security-observe-guard.md) (status
  `blocked`) covers bounded observation without secret persistence.
- **Enforcement today:** `ActionHarness` is Allow because the capability was
  granted on the command line and output is sanitized at the module boundary
  (`internal/permission/gate.go:135-141`, `internal/diag/sanitize.go:13-30`).
- **Public-boundary check:** `go test ./internal/diag/` (sanitize, audit,
  budget tests). LSP diagnostics mirror workspace file content already covered
  by C1's read capability.
- **Residual gap:** diagnostics content is configuration-derived and sanitized;
  no evidence gap beyond C1/C9 inheritance. Debug logs on disk inherit C1.
- **Classification:** ready (narrow, developer-mode-only, sanitized); LSP
  diagnostics — ready (no capability beyond file read).

**D7 surface with no implementation:** D7 also names *incident views* among the
surfaces to protect. No incident-view implementation exists in this tree
(`web-research-30-incident-viewer` is `blocked`); there is nothing to bypass
yet, and any future viewer lands behind human-only review (D7/D12), so it is
recorded here as considered-but-absent rather than as a channel.

## Platform matrix

Derived from repository configuration, not assumption:

- **Release builds** (`.goreleaser.yaml:10-28`): `linux/amd64`, `linux/arm64`,
  `darwin/amd64`, `darwin/arm64`, `windows/amd64`. `windows/arm64` is explicitly
  ignored (`.goreleaser.yaml:26-28`). CGO disabled; one binary, `./cmd`.
- **CI** (`.github/workflows/ci.yml:71-84`): tests run on `ubuntu-latest` and
  `macos-latest` only; the file itself records why there is **no Windows test
  leg** ("No Windows leg: OS-specific test semantics … kept the leg red"). The
  lint job cross-compiles for Windows (`make build-windows`) so Windows code at
  least compiles at CI time (`.github/workflows/ci.yml:43-46`).
- **Build-tagged code:** `internal/proc` (`process_unix.go` `!windows`,
  `process_windows.go`), `internal/atomicfile` (`nofollow_unix.go`,
  `nofollow_other.go`), `internal/notify` (`sender_darwin.go`,
  `sender_linux.go`, `sender_other.go`), `internal/lsp` (`exec_unix.go`,
  `exec_windows.go`, `config_unix.go`, `config_windows.go`),
  `internal/session` (`ownership_unix.go`, `ownership_windows.go`),
  `internal/components/app` (`resume_unix.go`, `resume_windows.go`).
  `internal/permission` and `internal/tools/webtool` carry no platform-tagged
  files — the gate and web framing are identical on every platform.

Per-platform classification for the D15/T30 question (real isolation evidence or
explicit disabled capability):

| Platform | Shipped | CI-tested | Isolation evidence | Classification |
| --- | --- | --- | --- | --- |
| linux/amd64 | yes | yes | none — process-group reaping in `internal/proc` is lifecycle, not sandboxing | unproved |
| linux/arm64 | yes | no (CI runs one linux arch) | none | unproved |
| darwin/amd64 | yes | yes (macos-latest is arm64 hardware; amd64 binary untested even on CI) | none | unproved |
| darwin/arm64 | yes | yes | none | unproved |
| windows/amd64 | yes | no — cross-compile only, no test leg | none; Git-Bash shell resolution `internal/tools/bashtool/shellconfig.go:3-16`; case-insensitive path matching handled at `internal/permission/rules.go:203-215` | unproved (weakest evidence: no test leg) |

No platform has any enforceable process/filesystem/network boundary today, so no
platform can claim protected-web readiness; per D15 the restricted mode must
disable unmediated channels (C3, C4, C6 at minimum) on **all five** builds, and
that restricted mode does not yet exist in code.

## Missing implementation blockers (task IDs)

Status read from the task notes in this tree at `7162d11f`:

| Gate | Task | Status today | What is missing |
| --- | --- | --- | --- |
| I | [security-process-isolation-design](../obsidian-tasks/security-process-isolation-design.md) | `todo` (design, unstarted) | the design itself, then concrete per-channel/per-platform isolation implementations with proof — none exist as task IDs yet |
| I-adjacent | [refactor-external-binary-runner](../obsidian-tasks/refactor-external-binary-runner.md) | `todo` | managed subprocess seam (argv without shell, process-tree termination) |
| I-adjacent | [sandbox-mcp-stdio-environment](../obsidian-tasks/sandbox-mcp-stdio-environment.md) | `todo` | minimal MCP child environment, 0600 config/logs, sandbox degradation |
| H | *(no task)* | **owner unidentified** | cross-process/cross-session web block and release coordination; closest seams are per-process only (`internal/session/ownership.go`, `internal/job/manager.go`) |
| P | [security-durable-provenance](../obsidian-tasks/security-durable-provenance.md) | `blocked` | provenance surviving turns, resume, fork, compaction, child outcomes |
| E | [security-model-egress](../obsidian-tasks/security-model-egress.md) | `blocked` | mediation of effective model requests per recipient |
| G | [enforce-agent-dataflow-policy](../obsidian-tasks/enforce-agent-dataflow-policy.md) | `blocked` | recipient-bound grants on actual final arguments |
| S | [security-storage-design](../obsidian-tasks/security-storage-design.md) + [security-encrypted-sessions](../obsidian-tasks/security-encrypted-sessions.md) | `todo` (design) / `blocked` | encrypted protected persistence with OS-backed keys |
| A | [routing-openai-account-admission](../obsidian-tasks/routing-openai-account-admission.md) | `blocked` | shared account admission foundation |
| R | [routing-priority-scheduling](../obsidian-tasks/routing-priority-scheduling.md) + [routing-human-exceptions](../obsidian-tasks/routing-human-exceptions.md) | `blocked` / `blocked` | priority admission and user-pin exceptions |
| eval | [agent-security-adversarial-evals](../obsidian-tasks/agent-security-adversarial-evals.md) | `todo` | reproducible attack/benign evaluation harness |

Downstream web tickets gated on this inventory per
[protected-web-research-tickets.md](protected-web-research-tickets.md):
web-research-36 (format feasibility, blocked by 01) and web-research-41
(restricted-mode eligibility, blocked by 01 and I). The unproved channels below
must be recorded in web-research-41's restricted mode as disabled until I
implementations land.

## Unproved register — missing evidence named

| Channel | Missing evidence |
| --- | --- |
| C1 file reads | No deny coverage or test proving `~/.cozyphi/web`, `~/.cozyphi/jobs`, `~/.cozyphi/session/**` unreachable by `read`; web cache storage format unknown (external module `cozy-tools v0.2.0`) |
| C2 file writes | No provenance binding of written content; no control on planting execution payloads (hooks, skills) under workspace paths |
| C3 shell/Python | No process/filesystem/environment/network isolation at all; no restricted mode that disables the channel; taint reset per turn unaddressed |
| C4 MCP | No untrusted framing of server output; no child environment minimization (`internal/mcp/stdio.go:148`); no server-process sandbox |
| C5 hooks | No consent/trust step for project-shipped hooks; no framing of `PreResult.Context`/`PostResult.Output` reaching the model; no hook-process isolation |
| C6 watches | No isolation of the watched command; single approval buys indefinite repeated egress-capable execution |
| C7 sub-agents | No durable provenance on child summaries (P blocked); job artifacts readable via C1 |
| C8 model egress | No recipient mediation (E blocked); durable provenance beyond one turn absent (P blocked) |
| C9 persistence | No provenance on compaction summaries or memory facts; no encryption at rest (S design `todo`, implementation `blocked`) |
| Platforms | No isolation evidence on any of the five shipped builds; no Windows test leg in CI; no restricted-mode implementation |

## Verification performed for this document

Docs-only verification per the task contract (no Go gates run):

1. Local relative-link resolution — all 16 unique markdown link targets in this file
   (task notes under `../obsidian-tasks/`, the two spec documents) were checked
   with `grep -oE` extraction of link targets from this file followed by
   `test -f` on each path resolved from `specs/`. All resolve.
2. `git diff --check` — clean (no whitespace errors).
3. File:line references were taken from the files at `7162d11f` as read during
   this investigation; they drift with future commits and should be re-checked
   by consumers.
