# Claude Code plugins

Status: implemented (2026-09-24). Task: `obsidian-tasks/claude-plugins.md`.

CozyPhi loads the skills and session hooks of Claude Code plugins, so a
plugin installed once through Claude Code (`/plugin install`) works in both
harnesses. The driving case is `superpowers`: 15 skills plus a `SessionStart`
hook that injects its `using-superpowers` bootstrap into the conversation.

## Scope

Supported plugin components:

| Component | Support |
| --- | --- |
| `skills/` (and manifest `skills` paths) | Loaded into the skill catalog under the plugin's namespace |
| `hooks/hooks.json` (and manifest `hooks` path) | `SessionStart` and `SessionEnd` only |
| `.claude-plugin/plugin.json` | Read for `name`, `skills`, `hooks`; optional |

Present but unsupported — each yields one warning per plugin, never an error:
`commands/`, `agents/`, `.mcp.json`, `.lsp.json`, `output-styles/`,
`monitors/`, hook events other than `SessionStart`/`SessionEnd`, and hooks
whose command references `${user_config.*}`.

Out of scope: installing, updating or removing plugins (Claude Code's
`/plugin` owns that), marketplaces, and any trust layer beyond "enabled in
Claude Code settings or listed in config".

## Configuration

```yaml
# ~/.cozyphi/config.yaml
plugins:
  enabled: true          # default true
  claude_dir: ~/.claude  # default; Claude Code's home, read-only for cozyphi
  paths:                 # local plugins; each entry is a plugin root, always enabled
    - ~/src/my-plugin
```

- `COZYPHI_PLUGINS=off` disables plugin loading for the process, like
  `COZYPHI_HOOKS=off` does for hooks.
- `~` expands to the home directory. A relative entry in `paths` is skipped
  with a warning: the global config has no project to be relative to.
- `plugins` is owner data: it lives only in the global config, never in a
  project's `.cozyphi/`.
- Below, "project root" means `Project.CheckoutRoot()`: the checkout the
  session runs in, or the working directory outside Git.

## Discovery

New package `internal/plugin`:

```go
type Plugin struct {
    ID        string   // "superpowers@claude-plugins-official"; paths plugins: Name
    Name      string   // namespace for skills and hook names: "superpowers"
    Root      string   // installPath or the configured path; ${CLAUDE_PLUGIN_ROOT}
    DataDir   string   // ${CLAUDE_PLUGIN_DATA}; created on first hook run
    SkillDirs []string // skills/ plus manifest extras; Root itself for a root SKILL.md
    HookFiles []string // hooks/hooks.json plus a manifest hooks path
}
```

Manifest `skills` and `hooks` accept a path or an array of paths relative to
`Root`; they add to the default locations. An inline hooks object in the
manifest is skipped with a warning.

```go

type Warning struct {
    Plugin string // ID, or the path when no ID could be read
    Msg    string // says what is wrong and what to do
}

func Discover(cfg Config, projectRoot string) ([]Plugin, []Warning)
```

`Discover` never returns an error: a missing directory means no plugins, and
every malformed input becomes a `Warning`.

**Claude-installed plugins** (`claude_dir`):

1. Read `<claude_dir>/plugins/installed_plugins.json`. Only `"version": 2` is
   understood; any other version yields one warning and zero plugins from
   this source. `plugins` maps `name@marketplace` to an array of install
   entries; each entry's `installPath` is the plugin root. An entry carrying
   `projectPath` counts only when it names the same directory as the project
   root (compared with `os.SameFile`, not as strings).
2. Resolve `enabledPlugins` by merging, later wins per key:
   `<claude_dir>/settings.json` < `<project>/.claude/settings.json` <
   `<project>/.claude/settings.local.json`. A plugin is enabled only when its
   final value is `true`; a missing key means disabled.
3. `Name` is the part of the ID before `@`. `DataDir` is
   `<claude_dir>/plugins/data/<id with every character outside [A-Za-z0-9_-]
   replaced by "-">/` — the directory Claude Code uses, so both harnesses
   share plugin state (observed: `superpowers@claude-plugins-official` →
   `superpowers-claude-plugins-official`).

**Local plugins** (`paths`): each path is a plugin root and is always enabled.
`Name` comes from `.claude-plugin/plugin.json` `name`, else the directory's
base name; `ID` equals `Name`; `DataDir` is `~/.cozyphi/plugins/data/<Name>/`.
A path that does not exist or is not a directory yields a warning.

Two enabled plugins with the same `Name` keep the first found (Claude-installed
before `paths`, in config order) and warn about the second.

## Skills catalog

The single `skill_path` string threaded through ~10 call sites becomes one
deep type in `internal/llm/skills`:

```go
type Source struct {
    Dir       string
    Namespace string            // "" for skill_path; plugin Name otherwise
    Vars      map[string]string // CLAUDE_PLUGIN_ROOT, CLAUDE_PLUGIN_DATA, CLAUDE_PROJECT_DIR
}

type Sources []Source

func (Sources) Load() ([]*Skill, error)
func Find(list []*Skill, name string) (*Skill, error) // was: returns *Skill
```

- `Load` re-reads disk on every call; there is no cache to invalidate. A
  missing source directory contributes nothing.
- `SkillPath string` is replaced by `Skills skills.Sources` in
  `llm.ModelConfig`, the engine, `prompt.Options`, `agenttool` and
  `commands.Host`. `cmd` assembles the list once: `skill_path` first, then one
  source per plugin skill directory.
- A plugin skill's name is `<namespace>:<name>`, e.g.
  `superpowers:brainstorming`. The same bare name may exist in several
  sources.
- `Find`: exact name, then case-insensitive name, then a bare name — the part
  after `:` or the skill's directory base name, as today — that matches
  exactly one skill. An ambiguous bare name returns an error listing the
  candidates; no match returns `nil, nil`, as the `nil` result does today.
- The prompt catalog format is unchanged (`### name`, description,
  `**Location:**`); the model reads `SKILL.md` through `read`.
- `${CLAUDE_PLUGIN_ROOT}`, `${CLAUDE_PLUGIN_DATA}` and `${CLAUDE_PROJECT_DIR}`
  are substituted from `Vars` wherever cozyphi inlines a skill body (plan-step
  skills, `agent_spawn` skills). A body the model reads itself is raw; the
  tool-mapping note (below) tells it what the placeholders mean.
- The scanner follows directory symlinks, guarded against cycles by resolved
  real path. The existing walk of `skill_path` is otherwise unchanged.
- A `SKILL.md` reached by two sources, by resolved real path, is listed once,
  in the first source's slot. A plugin's copy outranks `skill_path`'s, so a
  `skill_path` aimed at a plugin's own skills directory keeps the plugin's
  name and `Vars`; between two plugins, the first keeps it.
- Only `name` and `description` frontmatter are required; unknown keys are
  ignored, as today.
- Sub-agents receive the same catalog as their parent.

## Hooks

A second adapter behind the existing `hooks.Hook` seam, next to
`CommandHook`, in `internal/hooks/claude.go`:

```go
// Discover reads user and project hook manifests plus each enabled plugin's
// hooks.json (already resolved to files and Vars by internal/plugin), and
// returns every runnable entry plus every warning. hooks does not import
// internal/plugin: a plugin's hook files reach it only as PluginHooks.
func Discover(userDir, projectDir string, plugins ...PluginHooks) ([]Discovered, []Warning, error)

// PluginHooks describes one plugin's hook files in the terms hooks needs.
type PluginHooks struct {
    Name  string            // plugin name: the hook-name namespace and the source label
    Files []string          // absolute hooks.json paths
    Vars  map[string]string // CLAUDE_PLUGIN_ROOT, CLAUDE_PLUGIN_DATA, CLAUDE_PROJECT_DIR
}

type ClaudeHook struct { /* implements Hook; only Session does work */ }
```

- **Assembly.** Where the controller builds entries from `hooks.Discover`,
  plugin entries are appended. The merge is additive: a plugin hook never
  shadows a user hook by name. Entry names read
  `plugin:<Name>/<Event>#<n>`; the **hooks → list** palette command (`Ctrl+K`)
  shows the source as `plugin:<Name>`. `COZYPHI_HOOKS=off` and fail-closed-only mode apply to
  plugin hooks too.
- **Events.** `SessionStart` maps to `session_start`, `SessionEnd` to
  `session_shutdown`. Every other event is skipped with an "unsupported event"
  warning.
- **Source mapping.** cozyphi reasons become Claude's `source`:

  | cozyphi reason | `SessionStart.source` | `SessionEnd.reason` |
  | --- | --- | --- |
  | `startup` | `startup` | — |
  | `new` | `clear` | `clear` |
  | `resume` | `resume` | `resume` |
  | `compact` (new) | `compact` | — |
  | `quit` | — | `prompt_input_exit` |
  | anything else | not run | `other` |

- **Matcher.** Applied to `source`. Empty or `*` matches everything;
  otherwise it is an anchored regular expression, so `startup|clear|compact`
  behaves as in Claude Code. An invalid expression skips that matcher group
  with a warning.
- **Execution.** A hook with `args` runs as exec without a shell; otherwise
  `bash -c <command>`. `${CLAUDE_PLUGIN_ROOT}`, `${CLAUDE_PLUGIN_DATA}` and
  `${CLAUDE_PROJECT_DIR}` are substituted in `command`/`args` and also set in
  the environment; `DataDir` is created before the first run. The working
  directory is the project root. The environment passes through the existing
  `sanitizeEnv`, so secrets never reach the hook. `timeout` (seconds) is
  honored up to the existing 60 s cap; the default is 30 s — Claude Code
  waits up to 600 s, but cozyphi runs `session_start` synchronously while the
  session opens. `async: true` runs detached and its output is discarded.
- **Stdin.** One JSON object: `session_id`, `cwd`, `hook_event_name`, and
  `source` (`SessionStart`) or `reason` (`SessionEnd`). `transcript_path` is
  omitted: cozyphi's session file is not a Claude transcript, and a script
  parsing it as one would break.
- **Output.** Exit 0: stdout that parses as JSON supplies
  `hookSpecificOutput.additionalContext` (context) and `systemMessage` (toast);
  any other non-empty stdout is context verbatim. Context is capped at 16 KiB
  with a truncation marker. Any non-zero exit, including 2, is non-blocking:
  the first line of stderr becomes a warning and a debug log line.
- **Types.** `SessionResult` and `SessionOutcome` gain `Context string`;
  `mergeSessionUI` concatenates contexts in entry order.
- **Terminology.** The existing `~/.cozyphi/hooks/**/plugin.json` files are
  called "hook manifests" in docs and UI from now on; the file name stays for
  compatibility. "Plugin" means a Claude Code plugin only. `doc/hooks.md`
  gains a section on Claude plugin hooks and the new `compact` reason.

## Context delivery

- After `emitSessionStart`, the controller calls
  `eng.QueueSessionContext(out.Context)`. The engine parks the text, wrapped
  in `<system-reminder>`, and delivers it exactly once on whichever boundary
  comes first — the next composed user prompt (`composeUserPrompt`) or the
  next tool result (the executor drain `compactAdvice` already uses).
- The reminder stays in model history, so it survives resume; TUI replay and
  session titles strip it like every other reminder. `superpowers` does not
  match `resume`, so a resumed session is not bootstrapped twice.
- **Compaction.** After a successful `runCompaction` (overflow recovery,
  manual `/compact`, and the compaction the model requests through the
  `context` tool), the engine runs `hooks.SessionStart` with reason `compact`
  and queues the returned context. A successful user trim of the context
  (`Engine.TrimContextFrom`, the `/context` browser's trim action) does the
  same: dropping the first user message drops a plugin bootstrap with it, so
  a successful trim also re-fires `session_start` with reason `compact`, in
  the primary engine only. Unlike compaction, the trim refire runs in the
  background (`go engine.refireSessionStart(ctx)`) because trim is a UI
  action and waiting for a hook (up to 60 s) would freeze the screen; its
  context lands at the first prompt or tool-result boundary after the hook
  finishes, same as any other queued context. Toast and status from either
  refire are logged only: the engine has no UI channel. Trade-off: existing
  cozyphi `session_start` hooks now also see `compact`; this extends their
  contract and is documented and noted in the changelog, in exchange for one
  lifecycle and one contract.
- **Sub-agents.** An engine with `ParentID != ""` neither queues session
  context nor fires `compact`. Children get the skill catalog but no
  bootstrap, as in the opencode adapter.
- **Headless.** Lifecycle hooks fire only from the TUI controller today; no
  new entry point is added. Headless runs see plugin skills but receive no
  bootstrap.
- **Known limitation: cancelled compaction.** `runCompaction` runs the
  `compact` refire synchronously on the compaction's own `ctx`. Cancelling
  (Esc) while that hook run is still in flight cancels the hook too: the run
  returns no context (`ClaudeHook.run` reports "hook … cancelled" and
  `refireSessionStart`'s empty-context guard then queues nothing), so that
  refire's context is dropped. The plugin's `session_start` hook is
  stateless, though, so the bootstrap is not lost for good — it comes back at
  the next successful compaction, which runs with a fresh, uncancelled `ctx`.

## Tool mapping note

When at least one loaded skill came from a plugin source, the skills block of
the system prompt gains a short note: plugin skills are written for Claude
Code, and their tool names map as follows where the engine has that tool; a
step that needs one it lacks is skipped or done with the tools it does have.
`${CLAUDE_PLUGIN_ROOT}` is the plugin root — the directory above the skill's
`skills/` folder.

| Claude Code | cozyphi |
| --- | --- |
| `Skill` | `read` the `SKILL.md` at the listed Location |
| `Task` / `Agent` | `agent_spawn`, then `agent_wait` |
| `TodoWrite` | `plan` |
| `Bash`, `Read`, `Write`, `Edit`, `Grep` | `bash`, `read`, `write`, `edit`, `grep` |
| `Glob` | `find` |
| `WebFetch`, `WebSearch` | `web` |

## Errors and observability

- Nothing plugin-related stops a session from opening. Malformed JSON, an
  unknown `installed_plugins.json` version, a missing path, an invalid
  matcher and a failing hook each become a warning naming the file and the
  fix.
- Warnings appear in **hooks → list** and in the debug log (`COZYPHI_DEBUG`).
- At startup the debug log records each loaded plugin with its skill and
  hook counts.

## Testing

Tests live beside the code and use public interfaces only.

- `plugin.Discover` against temporary `claude_dir` trees: version 2,
  `projectPath` filtering, `enabledPlugins` layering, local `paths`, name
  clashes, unknown version, garbage JSON, unsupported components.
- `skills.Sources`: namespacing, bare-name resolution and ambiguity, symlinked
  directories and cycles, placeholder substitution in inlined bodies.
- `ClaudeHook` with fake scripts: plain and JSON stdout, `args` versus
  `shell`, matcher, timeout, exit 2 and other codes, context truncation,
  environment and substitution.
- Engine: queued context is delivered exactly once, through the prompt or the
  tool-result boundary; compaction re-fires `SessionStart`; a child engine
  stays silent.

## Code touched

- New: `internal/plugin/` (discovery), `internal/hooks/claude.go`.
- Changed: `internal/llm/skills` (`Sources`, `Find`, symlinks),
  `internal/project/config.go` (`plugins` section, `COZYPHI_PLUGINS`),
  `internal/llm/types.go`, `internal/agent` (engine, prompt, runner, plan
  actions, compaction, session-context queue), `internal/tools/agenttool`,
  `internal/tui/commands`, `internal/tui/sessions/view.go`,
  `internal/tui/controller/controller.go`, `internal/hooks` (types, manager
  merge, list output), `cmd/` assembly.
- Docs: this file, `doc/hooks.md`, `doc/project-layout.md`, `CHANGELOG.md`.

## Decisions made during implementation

- Plugin discovery runs in `project.LoadConfig`, not `cmd`, which yields
  `Config.Skills` and `Config.PluginHooks`. **hooks → reload** re-reads
  `hooks.json` but not the plugin set, which a restart refreshes. Saving
  settings also calls `RefreshProjectConfig` (`controller.go`,
  `sessions/view.go`), which re-runs `LoadConfig` and so rediscovers plugins
  without a restart: **hooks → list** warnings and the next **hooks → reload**
  reflect the new set right after a save. What stays on the old set until an
  explicit **hooks → reload** or a new session is the already-running engine's
  skill sources (baked into `llm.ModelConfig.Skills` at construction) and the
  controller's already-swapped-in `hooksManager`.
- Among install entries, one whose `projectPath` is the project root wins.
  Otherwise the first entry without `projectPath` wins, and other projects'
  entries never count. A plugin name must match `^[A-Za-z0-9][A-Za-z0-9._-]*$`,
  and manifest paths stay inside the plugin root.
- `skills.Sources.Load` returns the partial list together with the joined
  error. Callers fail only when nothing loaded.
- Context is accepted from `hookSpecificOutput.additionalContext`,
  `additionalContext` or `additional_context`. `QueueSessionContext` replaces
  rather than appends.
- Only engines built with `EngineOpts.LifecycleHooks` (controller primaries)
  accept context or re-fire on compaction. A child controller still runs the
  hook, but its engine ignores the result.
- In the shell form, placeholders reach the command through the environment
  (bash expands them), not through text substitution of `command`. This is
  injection-safe: a path containing quotes or `$()` cannot rewrite the
  command that runs. Only `args` are substituted textually, which keeps the
  same guarantee for the exec form.
- The first line of stderr is redacted before it reaches the debug log.
  Runtime hook failures (non-zero exit, timeout, cancellation) also surface
  as warnings in **hooks → list**, via `hooks.Manager.Failures()`, in addition to
  the debug log — a plugin hook never blocks a session, so without this its
  failure would reach the debug log only. A later successful run clears the
  warning. `exec.ErrWaitDelay` after a clean exit counts as success.
- The 16 KiB plugin context cap is separate from the 4 KiB `MaxContextBytes`
  cap of hook manifests.
- `session_start` runs synchronously, bounded by the hook timeout (30 s by
  default, 60 s max). Headless runs get plugin skills but no session hooks.
- The harness diag view drops the `plugin:` origin: `observedOrigins` in
  `internal/hooks/observe.go` maps discovery's source labels onto
  `diag.HookOrigin`'s closed vocabulary (`user`, `project`), and a
  `plugin:<Name>` source falls into its `default: continue` — present in
  `hooks list` but invisible to that diagnostic view. The config coverage
  inventory (`configCoverage`/`envCoverage` in
  `internal/tui/controller/developer_mode_coverage_test.go`) maps the
  `plugins` section, its `enabled`/`claude_dir`/`paths` keys and
  `COZYPHI_PLUGINS` to the skills key, `diag.KeyContextSkills`.
- The executor setter is renamed to `SetReminderDrain`, because it now
  carries session context as well.
- Skill bodies are expanded at load. The file the model `read`s stays raw.
- `Skill.Namespace` records the owning plugin (empty for `skill_path`). The
  Claude Code tool-mapping note keys on it, not on a `:` in the name, so a
  user skill named `team:s` does not turn the note on.
- A compact refire that returns no context leaves an undelivered
  startup/resume bootstrap in place; it does not clear it.
  `refireSessionStart` only calls `QueueSessionContext` when the hook
  actually returned non-empty context, so a `compact` run with nothing to add
  — a plugin with no `SessionStart` hook, or one whose matcher does not match
  `compact` — never overwrites context still waiting for delivery.
- Plugin discovery warnings (unsupported components such as `commands/`) show
  in **hooks → list** even with `COZYPHI_HOOKS=off`: that switch empties
  `hooks.Discover`'s own result, but `Controller.ListHooks` still adds
  `cfg.PluginWarnings()`, which `project.LoadConfig` collected independently
  of hook discovery. `COZYPHI_PLUGINS=off` is the switch that disables plugin
  discovery itself. **hooks → reload** counts only `hooks.json` warnings, since
  `ReloadHooks` does not consult `cfg.PluginWarnings()`.
