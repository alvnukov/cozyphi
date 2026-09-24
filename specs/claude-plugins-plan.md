# Claude Code Plugins Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** cozyphi loads the skills and `SessionStart`/`SessionEnd` hooks of Claude Code plugins. It finds them in Claude Code's own install records and in a configurable list of local plugin directories.

**Architecture:**
- A new leaf package, `internal/plugin`, turns `installed_plugins.json`, Claude settings and `plugins.paths` into plugin descriptions. Nothing in it fails a session.
- `internal/project` runs discovery once per `LoadConfig` and publishes two things: `Config.Skills` (a `skills.Sources` list) and `Config.PluginHooks`.
- The skill catalog becomes `skills.Sources` everywhere a single `SkillPath` string used to travel.
- Plugin hooks are a second `hooks.Hook` adapter, `ClaudeHook`, next to `CommandHook`.
- The engine delivers the hook's context exactly once as a `<system-reminder>`, and delivers it again after each compaction.

**Tech Stack:** Go 1.26, standard library only (`encoding/json`, `os/exec`, `regexp`, `maps`, `slices`, `cmp`). No new direct dependencies.

**Spec:** `doc/plugins.md` (commit `a5a9d3ca`). Task registry entry: `obsidian-tasks/claude-plugins.md`.

## Global Constraints

- Work only in `/Users/zol/src/cozyphi/.worktrees/claude-plugins`, branch `feature/claude-plugins`. Never `cd`: use `go -C <worktree>`, `git -C <worktree>` and absolute paths. The main checkout stays on `main`.
- Commits are signed (`-S`) Conventional Commits: English, lowercase, imperative, ≤72 chars. No `Co-authored-by`, `@mentions` or `fixes #`. Never push, open a PR or merge without explicit permission from the user.
- Run `make -C /Users/zol/src/cozyphi/.worktrees/claude-plugins fmt` before every commit. Comments are full sentences ending with a period (godot).
- `testing` and `testify` appear only in `*_test.go` files (depguard). Tests go through the public interface and live beside the code.
- Gates are scoped to the changed packages. Run exactly one scoped `golangci-lint` in Task 9; never sweep the whole repository.
- Before changing an existing function, run happ `code op='calls'` on it and read its callers. Before each commit, run happ `code op='diagnostics'` on the touched files.
- Naming:
  - A plugin skill is named `<plugin Name>:<skill name>`, e.g. `superpowers:brainstorming`.
  - A plugin hook entry is named `plugin:<Name>/<Event>#<n>`, and its source is `plugin:<Name>`.
- Plugin hooks:
  - Timeout: 30 s by default, capped at the existing 60 s `maxTimeout`.
  - Context is capped at 16 KiB. Truncation appends the marker `"\n[plugin hook context truncated at 16 KiB]"`.
  - Only `SessionStart` and `SessionEnd` are supported.
  - A failing plugin hook never blocks or denies a session. Its error goes to the debug log only.
- A hook's environment always passes through `sanitizeEnv`. `internal/hooks` never imports `internal/plugin`, and `internal/plugin` never imports `internal/agent`, `internal/session` or `internal/tools` (arch rule).
- Error and warning texts say what is wrong and what to do.
- A user-visible change adds a line under `## [Unreleased]` in `CHANGELOG.md` (Task 9).

## Review Focus

1. A hook that prints multi-line JSON and leaves a background child (`sleep 10 &`) holding stdout returns in well under 5 s, with its context intact. Test in Task 2 (`TestClaudeHookJSONContextSurvivesBackgroundChild`).
2. An install entry whose `projectPath` reaches the project root through a symlink still counts as this project's install. Test in Task 3 (`TestDiscoverProjectInstallMatchesThroughSymlink`).
3. A directory symlink cycle or a dangling symlink inside a skills directory does not stop loading, and the other skills are still found. Test in Task 1 (`TestSourcesFollowSymlinksAndSurviveCycles`).
4. Context queued twice before any boundary reaches the model once, and the second value replaces the first. Test in Task 7 (`TestQueuedSessionContextIsDeliveredOnce`).
5. A user skill `brainstorming` next to `superpowers:brainstorming`: an exact name wins, and a bare name shared by two plugins returns an error listing the candidates. Test in Task 1 (`TestFindPrefersExactThenRefusesAmbiguousBareName`).

---

### Task 1: Namespaced skill sources

**Files:**
- Create: `internal/llm/skills/sources.go`
- Create: `internal/llm/skills/sources_test.go`
- Modify: `internal/llm/skills/skills.go` (`Find` at 177-198; `LoadSkills` at 203-227 becomes a wrapper)
- Modify: `internal/llm/skills/skills_test.go:72-82` (`TestFind`)
- Modify the `Find` callers: `internal/agent/engine.go:1396` (`pendingSkillsInstruction`), `internal/agent/engine_plan_actions.go:323` (`queuePlanSkills`), `internal/agent/engine_runner.go:293` (`renderJobSkills`), `internal/tools/agenttool/skills.go:37` (`resolveSpawnSkills`)

**Interfaces:**
- Consumes: the existing `Skill`, `Parse`, `SkillFileName`.
- Produces:
  - `type Source struct { Dir, Namespace string; Vars map[string]string }`
  - `type Sources []Source`
  - `func (Sources) Load() ([]*Skill, error)`: returns the partial list plus the joined errors.
  - `func (Sources) String() string`
  - `func (Sources) Namespaced() bool`
  - `func Find(list []*Skill, name string) (*Skill, error)`
  - `LoadSkills(dir)` stays as a wrapper until Task 5 removes it.

- [ ] **Step 1: Write the failing tests**

`internal/llm/skills/sources_test.go`:

```go
package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeSkill plants dir/<sub>/SKILL.md with the given frontmatter name.
func writeSkill(t *testing.T, dir, sub, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, sub, SkillFileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	content := "---\nname: " + name + "\ndescription: " + name + " skill\n---\n" + body
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func names(list []*Skill) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		out = append(out, s.Name)
	}
	return out
}

func TestSourcesNamespacePluginSkills(t *testing.T) {
	user, plugin := t.TempDir(), t.TempDir()
	writeSkill(t, user, "brainstorming", "brainstorming", "user body")
	writeSkill(t, plugin, "brainstorming", "brainstorming", "plugin body")

	list, err := Sources{{Dir: user}, {Dir: plugin, Namespace: "superpowers"}}.Load()
	require.NoError(t, err)
	require.Equal(t, []string{"brainstorming", "superpowers:brainstorming"}, names(list))
	require.Equal(t, filepath.Join(plugin, "brainstorming"), list[1].Path)
}

func TestSourcesExpandVarsInBodiesOnly(t *testing.T) {
	dir := t.TempDir()
	file := writeSkill(t, dir, "tool", "tool", "run ${CLAUDE_PLUGIN_ROOT}/bin/x and ${UNKNOWN}")

	list, err := Sources{{
		Dir:       dir,
		Namespace: "p",
		Vars:      map[string]string{"CLAUDE_PLUGIN_ROOT": "/opt/p"},
	}}.Load()
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "run /opt/p/bin/x and ${UNKNOWN}", list[0].Body)

	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Contains(t, string(raw), "${CLAUDE_PLUGIN_ROOT}", "the file the model reads stays raw")
}

func TestSourcesFollowSymlinksAndSurviveCycles(t *testing.T) {
	root, target := t.TempDir(), t.TempDir()
	writeSkill(t, target, "linked", "linked", "via symlink")
	writeSkill(t, root, "plain", "plain", "plain")
	require.NoError(t, os.Symlink(target, filepath.Join(root, "link")))
	require.NoError(t, os.Symlink(root, filepath.Join(root, "plain", "loop")))
	require.NoError(t, os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "dangling")))

	list, err := Sources{{Dir: root}}.Load()
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"linked", "plain"}, names(list))
}

func TestSourcesDeduplicateTheSameFile(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "one", "one", "")
	link := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(dir, link))

	list, err := Sources{{Dir: dir}, {Dir: link, Namespace: "p"}}.Load()
	require.NoError(t, err)
	require.Equal(t, []string{"one"}, names(list), "the first source to reach a file owns it")
}

func TestSourcesReturnPartialListWithError(t *testing.T) {
	good := t.TempDir()
	writeSkill(t, good, "ok", "ok", "")
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	list, err := Sources{{Dir: file}, {Dir: good}, {Dir: filepath.Join(good, "absent")}, {}}.Load()
	require.ErrorContains(t, err, "is not a directory")
	require.Equal(t, []string{"ok"}, names(list))
}

func TestSourcesString(t *testing.T) {
	require.Equal(t, "(no skill directories)", Sources{}.String())
	require.Equal(t, "/a, /b", Sources{{Dir: "/a"}, {}, {Dir: "/b"}}.String())
	require.False(t, Sources{{Dir: "/a"}}.Namespaced())
	require.True(t, Sources{{Dir: "/a"}, {Dir: "/b", Namespace: "p"}}.Namespaced())
}

func TestFindPrefersExactThenRefusesAmbiguousBareName(t *testing.T) {
	user, a, b := t.TempDir(), t.TempDir(), t.TempDir()
	writeSkill(t, user, "brainstorming", "brainstorming", "")
	writeSkill(t, a, "tdd", "tdd", "")
	writeSkill(t, b, "tdd", "tdd", "")
	writeSkill(t, a, "only-here", "only-here", "")
	list, err := Sources{{Dir: user}, {Dir: a, Namespace: "alpha"}, {Dir: b, Namespace: "beta"}}.Load()
	require.NoError(t, err)

	got, err := Find(list, "brainstorming")
	require.NoError(t, err)
	require.Equal(t, "brainstorming", got.Name)

	got, err = Find(list, "ALPHA:TDD")
	require.NoError(t, err)
	require.Equal(t, "alpha:tdd", got.Name)

	got, err = Find(list, "only-here")
	require.NoError(t, err)
	require.Equal(t, "alpha:only-here", got.Name)

	_, err = Find(list, "tdd")
	require.ErrorContains(t, err, `skill "tdd" is ambiguous — use one of: alpha:tdd, beta:tdd`)

	got, err = Find(list, "nope")
	require.NoError(t, err)
	require.Nil(t, got)

	got, err = Find(list, "  ")
	require.NoError(t, err)
	require.Nil(t, got)
}
```

In `internal/llm/skills/skills_test.go`, replace `TestFind` (lines 72-82) with the two-value form:

```go
func TestFind(t *testing.T) {
	list := []*Skill{
		{Name: "Example Skill", Path: "/skills/example-skill"},
		{Name: "building-plugins", Path: "/skills/building-plugins"},
	}
	for _, name := range []string{"Example Skill", "example skill", "example-skill"} {
		got, err := Find(list, name)
		assert.NoError(t, err)
		if assert.NotNil(t, got, name) {
			assert.Equal(t, "Example Skill", got.Name)
		}
	}
	got, err := Find(list, "missing")
	assert.NoError(t, err)
	assert.Nil(t, got)
	got, err = Find(nil, "x")
	assert.NoError(t, err)
	assert.Nil(t, got)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins test ./internal/llm/skills/...`
Expected: build failure, `undefined: Sources` and `assignment mismatch: 2 variables but Find returns 1 value`.

- [ ] **Step 3: Implement `sources.go` and the new `Find`**

`internal/llm/skills/sources.go`:

```go
package skills

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Source is one directory the skill catalog is read from. The user's
// skill_path has no Namespace; a Claude Code plugin's skills directory
// carries the plugin name, so its skills read "<namespace>:<name>", and the
// placeholder values its bodies may reference.
type Source struct {
	Dir       string
	Namespace string
	// Vars maps a placeholder name (CLAUDE_PLUGIN_ROOT) to its value; every
	// "${NAME}" in a loaded body is replaced. The file on disk stays raw.
	Vars map[string]string
}

// Sources is the ordered catalog: skill_path first, then plugin directories.
// It is re-read on every Load; there is no cache to invalidate.
type Sources []Source

// Load reads every source in order. A file reached twice (a symlink, or two
// sources sharing a tree) belongs to the first source that reached it. A
// source that fails contributes its error and nothing else: the returned
// list holds every skill the other sources produced.
func (ss Sources) Load() ([]*Skill, error) {
	var (
		out  []*Skill
		errs []error
		seen = make(map[string]bool)
	)
	for _, src := range ss {
		found, err := src.load()
		if err != nil {
			errs = append(errs, err)
		}
		for _, skill := range found {
			key := skill.SkillFilePath
			if real, err := filepath.EvalSymlinks(key); err == nil {
				key = real
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, skill)
		}
	}
	return out, errors.Join(errs...)
}

// String lists the configured directories for error messages.
func (ss Sources) String() string {
	dirs := make([]string, 0, len(ss))
	for _, src := range ss {
		if src.Dir != "" {
			dirs = append(dirs, src.Dir)
		}
	}
	if len(dirs) == 0 {
		return "(no skill directories)"
	}
	return strings.Join(dirs, ", ")
}

// Namespaced reports whether any source is a plugin's, which is when the
// prompt needs the Claude Code tool-name mapping.
func (ss Sources) Namespaced() bool {
	return slices.ContainsFunc(ss, func(src Source) bool { return src.Namespace != "" })
}

func (src Source) load() ([]*Skill, error) {
	if src.Dir == "" {
		return nil, nil
	}
	st, err := os.Stat(src.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("skills: %w", err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf(
			"skills: %s is not a directory — point skill_path or the plugin at a directory", src.Dir)
	}
	w := walker{visited: make(map[string]bool)}
	w.walk(src.Dir)
	for _, skill := range w.found {
		src.adopt(skill)
	}
	return w.found, errors.Join(w.errs...)
}

// adopt applies the source's namespace and placeholder values to a parsed skill.
func (src Source) adopt(skill *Skill) {
	if src.Namespace != "" {
		skill.Name = src.Namespace + ":" + cmp.Or(skill.Name, filepath.Base(skill.Path))
	}
	skill.Body = expandVars(skill.Body, src.Vars)
}

func expandVars(body string, vars map[string]string) string {
	if len(vars) == 0 {
		return body
	}
	pairs := make([]string, 0, 2*len(vars))
	for _, k := range slices.Sorted(maps.Keys(vars)) {
		pairs = append(pairs, "${"+k+"}", vars[k])
	}
	return strings.NewReplacer(pairs...).Replace(body)
}

// walker is filepath.WalkDir plus directory symlinks. It remembers every
// directory by resolved real path, so a link back up the tree ends there
// instead of looping forever.
type walker struct {
	visited map[string]bool
	found   []*Skill
	errs    []error
}

func (w *walker) walk(dir string) {
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		w.errs = append(w.errs, fmt.Errorf("skills: %w", err))
		return
	}
	if w.visited[real] {
		return
	}
	w.visited[real] = true
	entries, err := os.ReadDir(dir)
	if err != nil {
		w.errs = append(w.errs, fmt.Errorf("skills: %w", err))
		return
	}
	for _, ent := range entries { // ReadDir sorts by name, as WalkDir did
		path := filepath.Join(dir, ent.Name())
		switch {
		case ent.IsDir():
			w.walk(path)
		case ent.Type()&fs.ModeSymlink != 0:
			// A dangling link, or one to a file, is not a skill directory.
			if st, err := os.Stat(path); err == nil && st.IsDir() {
				w.walk(path)
			}
		case ent.Name() == SkillFileName:
			if skill, err := Parse(path); err == nil {
				w.found = append(w.found, skill) // invalid files are skipped, as before
			}
		}
	}
}
```

In `skills.go`, replace `Find` and turn `LoadSkills` into a wrapper:

```go
// Find resolves a skill by name: exact, then case-insensitive, then a bare
// name — the part after "<namespace>:" or the skill's directory name — that
// matches exactly one skill. A bare name shared by several skills is an
// error naming them; no match is nil, nil.
func Find(list []*Skill, name string) (*Skill, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	for _, s := range list {
		if s.Name == name {
			return s, nil
		}
	}
	for _, s := range list {
		if strings.EqualFold(s.Name, name) {
			return s, nil
		}
	}
	var candidates []*Skill
	for _, s := range list {
		if strings.EqualFold(bareName(s.Name), name) || strings.EqualFold(filepath.Base(s.Path), name) {
			candidates = append(candidates, s)
		}
	}
	switch len(candidates) {
	case 0:
		return nil, nil
	case 1:
		return candidates[0], nil
	}
	full := make([]string, 0, len(candidates))
	for _, s := range candidates {
		full = append(full, s.Name)
	}
	return nil, fmt.Errorf("skill %q is ambiguous — use one of: %s", name, strings.Join(full, ", "))
}

// bareName strips a plugin namespace: "superpowers:tdd" → "tdd".
func bareName(name string) string {
	if _, bare, ok := strings.Cut(name, ":"); ok {
		return bare
	}
	return name
}

// LoadSkills reads one unnamespaced directory. It is kept until every
// caller carries Sources (see the Task 5 refactor).
func LoadSkills(skillDir string) ([]*Skill, error) {
	return Sources{{Dir: skillDir}}.Load()
}
```

Add `"fmt"` to the imports of `skills.go` if it is missing.

Update the four `Find` callers so they compile and keep today's behaviour:

`internal/agent/engine.go` (`pendingSkillsInstruction`):

```go
			if s, _ := skills.Find(list, name); s != nil && s.SkillFilePath != "" {
```

`internal/agent/engine_plan_actions.go` (`queuePlanSkills`):

```go
		skill, err := skills.Find(catalog, name)
		if err != nil {
			debuglog.Logf("plan: %v", err)
		}
		if skill != nil && skill.Body != "" {
```

`internal/agent/engine_runner.go` (`renderJobSkills`):

```go
		skill, err := skills.Find(catalog, name)
		if err != nil {
			return "", fmt.Errorf("agent: job skill: %w — re-spawn the job with one of the listed names", err)
		}
		if skill == nil {
```

`internal/tools/agenttool/skills.go` (`resolveSpawnSkills`):

```go
		skill, err := skills.Find(catalog, name)
		if err != nil {
			return nil, fmt.Errorf("agent_spawn: %w", err)
		}
		if skill == nil {
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins test ./internal/llm/skills/... ./internal/agent/... ./internal/tools/agenttool/...`
Expected: PASS.

- [ ] **Step 5: Format, diagnose, commit**

```bash
make -C /Users/zol/src/cozyphi/.worktrees/claude-plugins fmt
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins add internal/llm/skills internal/agent internal/tools/agenttool
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins commit -S -m "feat(skills): add namespaced skill sources"
```

---

### Task 2: Claude Code plugin session hooks

**Files:**
- Create: `internal/hooks/claude.go`
- Create: `internal/hooks/claude_test.go`
- Modify: `internal/hooks/types.go` (reason constants; `SessionResult.Context`)
- Modify: `internal/hooks/manager.go` (`SessionOutcome.Context` at 332; `mergeSessionUI` at 461)
- Modify: `internal/hooks/discover.go` (`Discovered.claude`; `Discover` gains `plugins ...PluginHooks`)
- Modify: `internal/hooks/load.go` (`Load` and `LoadObserved` gain `plugins ...PluginHooks`)
- Modify: `internal/hooks/command.go:50` (`EntryFromDiscovered`)

**Interfaces:**
- Consumes: the existing `sanitizeEnv`, `hookEnv`, `environ`, `limitedBuffer`, `MaxHookOutputBytes`, `maxTimeout`, `debuglog`, and `redact.Redact`.
- Produces:
  - Constants:
    - `EnvClaudePluginRoot = "CLAUDE_PLUGIN_ROOT"`
    - `EnvClaudePluginData = "CLAUDE_PLUGIN_DATA"`
    - `EnvClaudeProjectDir = "CLAUDE_PROJECT_DIR"`
    - `MaxPluginContextBytes = 16 * 1024`
    - `SourcePluginPrefix = "plugin:"`
    - `ReasonStartup`, `ReasonNew`, `ReasonResume`, `ReasonCompact`, `ReasonQuit`
  - `type PluginHooks struct { Name string; Files []string; Vars map[string]string }`
  - `func Discover(userDir, projectDir string, plugins ...PluginHooks) ([]Discovered, []Warning, error)`
  - `func Load(userDir, projectDir string, plugins ...PluginHooks) (*Manager, []Warning, error)`
  - `func LoadObserved(userDir, projectDir string, plugins ...PluginHooks) (*Manager, LoadFacts, []Warning, error)`
  - `SessionResult.Context` and `SessionOutcome.Context` (entry order, joined by `"\n\n"`)
  - `type ClaudeHook` (implements `Hook`)

- [ ] **Step 1: Write the failing tests**

`internal/hooks/claude_test.go` (package `hooks`, matching the other test files there):

```go
package hooks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/redact"
)

// claudePlugin plants a plugin root with hooks/hooks.json and returns the
// PluginHooks that describes it, as internal/plugin would.
func claudePlugin(t *testing.T, name, hooksJSON string) PluginHooks {
	t.Helper()
	root := t.TempDir()
	file := filepath.Join(root, "hooks", "hooks.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
	require.NoError(t, os.WriteFile(file, []byte(hooksJSON), 0o600))
	return PluginHooks{
		Name:  name,
		Files: []string{file},
		Vars: map[string]string{
			EnvClaudePluginRoot: root,
			EnvClaudePluginData: filepath.Join(t.TempDir(), "data"),
			EnvClaudeProjectDir: t.TempDir(),
		},
	}
}

// oneHook renders a hooks.json with one command hook under event/matcher.
func oneHook(t *testing.T, event, matcher string, hook map[string]any) string {
	t.Helper()
	hook["type"] = "command"
	raw, err := json.Marshal(map[string]any{"hooks": map[string]any{
		event: []any{map[string]any{"matcher": matcher, "hooks": []any{hook}}},
	}})
	require.NoError(t, err)
	return string(raw)
}

func pluginManager(t *testing.T, plugins ...PluginHooks) *Manager {
	t.Helper()
	t.Setenv(EnvHooks, "")
	mgr, warns, err := Load("", "", plugins...)
	require.NoError(t, err)
	require.Empty(t, warns)
	return mgr
}

// pluginHook returns the single hook the plugin declares, for tests that
// need the error a manager would only log.
func pluginHook(t *testing.T, p PluginHooks) Hook {
	t.Helper()
	t.Setenv(EnvHooks, "")
	found, warns, err := Discover("", "", p)
	require.NoError(t, err)
	require.Empty(t, warns)
	require.Len(t, found, 1)
	return EntryFromDiscovered(found[0]).Hook
}

func startWith(mgr *Manager, reason string) SessionOutcome {
	return mgr.SessionStart(context.Background(), SessionEvent{SessionID: "s1", Cwd: "/tmp", Reason: reason})
}

func TestClaudeHookJSONContextSurvivesBackgroundChild(t *testing.T) {
	cmd := `echo '{'; echo '"hookSpecificOutput": {"hookEventName": "SessionStart",'; ` +
		`echo '"additionalContext": "BOOT"},'; echo '"systemMessage": "hello"}'; sleep 10 &`
	mgr := pluginManager(t, claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{"command": cmd})))

	began := time.Now()
	out := startWith(mgr, ReasonStartup)
	require.Less(t, time.Since(began), 5*time.Second, "a background child must not hold the session open")
	require.Equal(t, "BOOT", out.Context)
	require.Equal(t, "hello", out.Toast)
}

func TestClaudeHookAcceptsEveryContextSpelling(t *testing.T) {
	for _, body := range []string{`{"additionalContext":"X"}`, `{"additional_context":"X"}`, `plain X`} {
		cmd := "printf '%s' '" + body + "'"
		mgr := pluginManager(t, claudePlugin(t, "demo", oneHook(t, "SessionStart", "*", map[string]any{"command": cmd})))
		require.Contains(t, startWith(mgr, ReasonStartup).Context, "X", body)
	}
}

func TestClaudeHookMatcherAppliesToMappedSource(t *testing.T) {
	mgr := pluginManager(t, claudePlugin(t, "demo",
		oneHook(t, "SessionStart", "startup|clear|compact", map[string]any{"command": "echo hit"})))
	for reason, want := range map[string]string{
		ReasonStartup: "hit", ReasonNew: "hit", ReasonCompact: "hit", ReasonResume: "", "reload": "",
	} {
		require.Equal(t, want, startWith(mgr, reason).Context, reason)
	}
}

func TestClaudeHookStdinCarriesClaudeFields(t *testing.T) {
	p := claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{
		"command": `cat > "$CLAUDE_PLUGIN_DATA/in.json"`,
	}))
	startWith(pluginManager(t, p), ReasonNew)

	raw, err := os.ReadFile(filepath.Join(p.Vars[EnvClaudePluginData], "in.json"))
	require.NoError(t, err, "the data dir is created before the first run")
	var in map[string]any
	require.NoError(t, json.Unmarshal(raw, &in))
	require.Equal(t, map[string]any{
		"session_id": "s1", "cwd": "/tmp", "hook_event_name": "SessionStart", "source": "clear",
	}, in)
}

func TestClaudeSessionEndMapsQuitReason(t *testing.T) {
	p := claudePlugin(t, "demo", oneHook(t, "SessionEnd", "", map[string]any{
		"command": `cat > "$CLAUDE_PLUGIN_DATA/end.json"`,
	}))
	pluginManager(t, p).SessionShutdown(context.Background(),
		SessionEvent{SessionID: "s1", Cwd: "/tmp", Reason: ReasonQuit})

	raw, err := os.ReadFile(filepath.Join(p.Vars[EnvClaudePluginData], "end.json"))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"reason":"prompt_input_exit"`)
	require.Contains(t, string(raw), `"hook_event_name":"SessionEnd"`)
}

func TestClaudeHookEnvironmentIsSanitizedAndRooted(t *testing.T) {
	t.Setenv("DEMO_API_KEY", "leak")
	p := claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{
		"command": `echo "key=${DEMO_API_KEY:-unset} root=$CLAUDE_PLUGIN_ROOT pwd=$(pwd -P)"`,
	}))
	project, err := filepath.EvalSymlinks(p.Vars[EnvClaudeProjectDir])
	require.NoError(t, err)

	got := startWith(pluginManager(t, p), ReasonStartup).Context
	require.Equal(t, "key=unset root="+p.Vars[EnvClaudePluginRoot]+" pwd="+project, got)
}

func TestClaudeHookArgsRunWithoutShell(t *testing.T) {
	p := claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{
		"command": "printf",
		"args":    []string{"%s|", "one two", "${CLAUDE_PLUGIN_ROOT}", "$HOME"},
	}))
	got := startWith(pluginManager(t, p), ReasonStartup).Context
	require.Equal(t, "one two|"+p.Vars[EnvClaudePluginRoot]+"|$HOME|", got, "no shell expansion, only placeholders")
}

func TestClaudeHookFailureIsNonBlockingAndRedacted(t *testing.T) {
	token := "ghp_" + strings.Repeat("a1B2", 9)
	p := claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{
		"command": "echo 'boom " + token + "' >&2; exit 2",
	}))

	_, err := pluginHook(t, p).Session(context.Background(),
		SessionEvent{Kind: KindSessionStart, SessionID: "s1", Cwd: "/tmp", Reason: ReasonStartup})
	require.ErrorContains(t, err, "exited 2")
	require.ErrorContains(t, err, redact.Marker)
	require.NotContains(t, err.Error(), token)

	out := startWith(pluginManager(t, p), ReasonStartup)
	require.False(t, out.Denied, "exit 2 never blocks a session")
	require.Empty(t, out.Context)
}

func TestClaudeHookTimeout(t *testing.T) {
	p := claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{"command": "sleep 5", "timeout": 1}))
	began := time.Now()
	_, err := pluginHook(t, p).Session(context.Background(),
		SessionEvent{Kind: KindSessionStart, Reason: ReasonStartup, Cwd: "/tmp"})
	require.ErrorContains(t, err, "timed out after 1s")
	require.ErrorContains(t, err, "raise its timeout")
	require.Less(t, time.Since(began), 4*time.Second)
}

func TestClaudeHookContextIsCappedOnARuneBoundary(t *testing.T) {
	p := claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{
		"command": `cat "$CLAUDE_PLUGIN_ROOT/big.txt"`,
	}))
	big := "a" + strings.Repeat("é", 9000) // é starts at odd offsets, so byte 16384 is mid-rune
	require.NoError(t, os.WriteFile(filepath.Join(p.Vars[EnvClaudePluginRoot], "big.txt"), []byte(big), 0o600))

	got := startWith(pluginManager(t, p), ReasonStartup).Context
	const marker = "\n[plugin hook context truncated at 16 KiB]"
	require.True(t, strings.HasSuffix(got, marker))
	body := strings.TrimSuffix(got, marker)
	require.True(t, utf8.ValidString(body))
	require.LessOrEqual(t, len(body), MaxPluginContextBytes)
	require.Greater(t, len(body), MaxPluginContextBytes-4)
}

func TestClaudeHooksWarnAndSkipWhatCozyphiCannotRun(t *testing.T) {
	t.Setenv(EnvHooks, "")
	p := claudePlugin(t, "demo", `{"hooks":{
	  "PreToolUse":[{"hooks":[{"type":"command","command":"true"}]}],
	  "SessionStart":[
	    {"matcher":"(","hooks":[{"type":"command","command":"true"}]},
	    {"hooks":[
	      {"type":"prompt","command":"true"},
	      {"type":"command","shell":"powershell","command":"true"},
	      {"type":"command","command":"  "},
	      {"type":"command","command":"echo ${user_config.token}"}
	    ]}
	  ]}}`)
	found, warns, err := Discover("", "", p)
	require.NoError(t, err)
	require.Empty(t, found)
	require.Len(t, warns, 6)
	text := make([]string, 0, len(warns))
	for _, w := range warns {
		text = append(text, w.String())
	}
	all := strings.Join(text, "\n")
	for _, want := range []string{
		"event PreToolUse is not supported", "not a valid regular expression", `hook type "prompt"`,
		`hook shell "powershell"`, "empty command", "${user_config.*}",
	} {
		require.Contains(t, all, want)
	}
}

func TestClaudeHooksInvalidJSONIsAWarning(t *testing.T) {
	t.Setenv(EnvHooks, "")
	found, warns, err := Discover("", "", claudePlugin(t, "demo", "{"))
	require.NoError(t, err)
	require.Empty(t, found)
	require.Len(t, warns, 1)
	require.Contains(t, warns[0].Message, "invalid hooks.json")
	require.Contains(t, warns[0].Message, "fix the file or disable the plugin")
}

func TestClaudeHookContextsJoinInEntryOrder(t *testing.T) {
	a := claudePlugin(t, "alpha", oneHook(t, "SessionStart", "", map[string]any{"command": "echo A"}))
	b := claudePlugin(t, "beta", oneHook(t, "SessionStart", "", map[string]any{"command": "echo B"}))
	require.Equal(t, "A\n\nB", startWith(pluginManager(t, a, b), ReasonStartup).Context)
}

func TestClaudeHookNamesAndSource(t *testing.T) {
	t.Setenv(EnvHooks, "")
	p := claudePlugin(t, "demo", oneHook(t, "SessionStart", "startup", map[string]any{"command": "true", "async": true}))
	found, _, err := Discover("", "", p)
	require.NoError(t, err)
	require.Len(t, found, 1)
	d := found[0]
	require.Equal(t, "plugin:demo/SessionStart#1", d.Manifest.Name)
	require.Equal(t, "plugin:demo", d.Source)
	require.Equal(t, "demo", d.Manifest.Plugin)
	require.Equal(t, KindSessionStart, d.Manifest.Kind)
	require.Equal(t, "startup", d.Manifest.Match)
	require.Equal(t, 30*time.Second, d.Manifest.Timeout)
	require.True(t, EntryFromDiscovered(d).Async)
}

func TestClaudeHooksHonourHooksOff(t *testing.T) {
	t.Setenv(EnvHooks, "off")
	found, warns, err := Discover("", "", claudePlugin(t, "demo", "{"))
	require.NoError(t, err)
	require.Empty(t, found)
	require.Empty(t, warns)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins test ./internal/hooks/ -run 'Claude'`
Expected: build failure, `undefined: PluginHooks`.

- [ ] **Step 3: Extend types, manager, discovery and load**

`internal/hooks/types.go`:
- Change the `SessionEvent.Reason` comment to `// startup | new | resume | compact | quit`.
- Add below `SessionEvent`:

```go
// Session lifecycle reasons, as SessionEvent.Reason carries them. compact is
// sent to session_start after a successful compaction.
const (
	ReasonStartup = "startup"
	ReasonNew     = "new"
	ReasonResume  = "resume"
	ReasonCompact = "compact"
	ReasonQuit    = "quit"
)
```

Add a field to `SessionResult`:

```go
	// Context is model-facing text a session_start hook contributes; the
	// engine delivers it once as a system reminder.
	Context string
```

`internal/hooks/manager.go`: add `Context string` to `SessionOutcome`, and extend `mergeSessionUI`:

```go
func mergeSessionUI(out *SessionOutcome, res SessionResult) {
	if res.Toast != "" {
		out.Toast = res.Toast
	}
	if res.StatusSet {
		out.Status = res.Status
		out.StatusSet = true
	}
	if res.Context != "" {
		if out.Context != "" {
			out.Context += "\n\n"
		}
		out.Context += res.Context
	}
}
```

`internal/hooks/discover.go`:
- Add to `Discovered`:

```go
	// claude is set for a Claude Code plugin hook; EntryFromDiscovered runs
	// it instead of a CommandHook.
	claude *ClaudeHook
```

- Change the signature and doc comment, and append the plugin hooks after the sort:

```go
// Discover loads plugin.json from userDir then projectDir, then appends the
// hooks of each Claude Code plugin in order. Plugin hooks never shadow a
// user or project hook: their names live under "plugin:<Name>/".
// ...keep the rest of the existing comment...
func Discover(userDir, projectDir string, plugins ...PluginHooks) ([]Discovered, []Warning, error) {
```

```go
	sort.Slice(out, func(i, j int) bool { /* unchanged */ })
	for _, p := range plugins {
		found, warns := parseClaudeHooks(p)
		out = append(out, found...)
		warnings = append(warnings, warns...)
	}
	return out, warnings, nil
```

`internal/hooks/load.go`:

```go
func Load(userDir, projectDir string, plugins ...PluginHooks) (*Manager, []Warning, error) {
	mgr, _, warns, err := LoadObserved(userDir, projectDir, plugins...)
	return mgr, warns, err
}
```

```go
func LoadObserved(userDir, projectDir string, plugins ...PluginHooks) (*Manager, LoadFacts, []Warning, error) {
	found, warns, err := Discover(userDir, projectDir, plugins...)
```

The rest stays as it is. `ObserveLoad` needs no change: `observedOrigins` already drops a source that is neither `user` nor `project`, and a plugin hook has no run path to leak.

`internal/hooks/command.go`, `EntryFromDiscovered`:

```go
func EntryFromDiscovered(d Discovered) Entry {
	if d.claude != nil {
		// A plugin hook is never fail-closed: in fail-closed-only mode it is
		// skipped, and a failure is logged, never a deny.
		return Entry{Hook: d.claude, Kind: d.Manifest.Kind, Async: d.Manifest.Async}
	}
	return Entry{ /* unchanged */ }
}
```

- [ ] **Step 4: Implement `claude.go`**

```go
package hooks

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alvnukov/cozyphi/internal/redact"
)

// Placeholders a Claude Code plugin hook may reference. They are set in the
// hook's environment and substituted in the args of an exec-style hook.
const (
	EnvClaudePluginRoot = "CLAUDE_PLUGIN_ROOT"
	EnvClaudePluginData = "CLAUDE_PLUGIN_DATA"
	EnvClaudeProjectDir = "CLAUDE_PROJECT_DIR"
)

// MaxPluginContextBytes caps the context one plugin hook contributes. It is
// larger than MaxContextBytes because a plugin bootstrap (superpowers'
// using-superpowers) is a whole skill body, not a note.
const MaxPluginContextBytes = 16 * 1024

// SourcePluginPrefix starts the Source of a plugin hook: "plugin:<Name>".
const SourcePluginPrefix = "plugin:"

const (
	// claudeDefaultTimeout is shorter than Claude Code's 600 s because
	// session_start runs synchronously while the session opens.
	claudeDefaultTimeout = 30 * time.Second
	pluginContextMarker  = "\n[plugin hook context truncated at 16 KiB]"
)

// PluginHooks describes one Claude Code plugin's hook files in the terms this
// package needs, so hooks never imports internal/plugin.
type PluginHooks struct {
	Name  string            // plugin name: the hook-name namespace and the source label
	Files []string          // absolute hooks.json paths
	Vars  map[string]string // EnvClaudePluginRoot, EnvClaudePluginData, EnvClaudeProjectDir
}

// claudeEvents maps the supported Claude Code events to cozyphi kinds.
var claudeEvents = map[string]Kind{
	"SessionStart": KindSessionStart,
	"SessionEnd":   KindSessionShutdown,
}

type claudeHooksFile struct {
	Hooks map[string][]claudeMatcherGroup `json:"hooks"`
}

type claudeMatcherGroup struct {
	Matcher string              `json:"matcher"`
	Hooks   []claudeHookCommand `json:"hooks"`
}

type claudeHookCommand struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Shell   string   `json:"shell"`
	Timeout float64  `json:"timeout"` // seconds
	Async   bool     `json:"async"`
}

func parseClaudeHooks(p PluginHooks) ([]Discovered, []Warning) {
	var (
		out    []Discovered
		warns  []Warning
		counts = make(map[string]int)
	)
	for _, file := range p.Files {
		warn := func(format string, args ...any) {
			warns = append(warns, Warning{Path: file, Message: fmt.Sprintf(format, args...)})
		}
		data, err := os.ReadFile(file)
		if err != nil {
			warn("read hooks file: %v — reinstall the plugin or fix the hooks path in its plugin.json", err)
			continue
		}
		var parsed claudeHooksFile
		if err := json.Unmarshal(data, &parsed); err != nil {
			warn("invalid hooks.json: %v — fix the file or disable the plugin", err)
			continue
		}
		for _, event := range slices.Sorted(maps.Keys(parsed.Hooks)) {
			kind, ok := claudeEvents[event]
			if !ok {
				warn("event %s is not supported; skipped — cozyphi runs SessionStart and SessionEnd only", event)
				continue
			}
			for _, group := range parsed.Hooks[event] {
				matcher, err := compileMatcher(group.Matcher)
				if err != nil {
					warn("%s matcher %q is not a valid regular expression: %v; group skipped — fix the matcher",
						event, group.Matcher, err)
					continue
				}
				for _, hc := range group.Hooks {
					if problem := claudeCommandProblem(hc); problem != "" {
						warn("%s: %s", event, problem)
						continue
					}
					counts[event]++
					name := fmt.Sprintf("%s%s/%s#%d", SourcePluginPrefix, p.Name, event, counts[event])
					hook := &ClaudeHook{
						name:    name,
						kind:    kind,
						matcher: matcher,
						command: hc.Command,
						args:    hc.Args,
						timeout: claudeTimeout(hc.Timeout),
						vars:    p.Vars,
					}
					out = append(out, Discovered{
						Manifest: Manifest{
							Name:    name,
							Kind:    kind,
							Match:   cmp.Or(group.Matcher, "*"),
							Run:     hc.Command,
							Timeout: hook.timeout,
							Async:   hc.Async,
							Plugin:  p.Name,
							Dir:     p.Vars[EnvClaudePluginRoot],
							Path:    file,
						},
						Source: SourcePluginPrefix + p.Name,
						claude: hook,
					})
				}
			}
		}
	}
	return out, warns
}

func claudeCommandProblem(hc claudeHookCommand) string {
	userConfig := func(s string) bool { return strings.Contains(s, "${user_config.") }
	switch {
	case hc.Type != "command":
		return fmt.Sprintf("hook type %q is not supported; skipped — cozyphi runs command hooks only", hc.Type)
	case hc.Shell != "" && hc.Shell != "bash":
		return fmt.Sprintf("hook shell %q is not supported; skipped — use bash or omit shell", hc.Shell)
	case strings.TrimSpace(hc.Command) == "":
		return "hook has an empty command; skipped — set command in hooks.json"
	case userConfig(hc.Command) || slices.ContainsFunc(hc.Args, userConfig):
		return "hook references ${user_config.*}, which cozyphi does not provide; skipped"
	}
	return ""
}

// compileMatcher anchors a Claude matcher, so "startup|clear" matches either
// word and nothing longer. Empty and "*" match everything (nil).
func compileMatcher(m string) (*regexp.Regexp, error) {
	if m == "" || m == "*" {
		return nil, nil
	}
	return regexp.Compile("^(?:" + m + ")$")
}

func claudeTimeout(seconds float64) time.Duration {
	if seconds <= 0 {
		return claudeDefaultTimeout
	}
	if seconds >= maxTimeout.Seconds() {
		return maxTimeout
	}
	return time.Duration(seconds * float64(time.Second))
}

// ClaudeHook runs one Claude Code plugin command hook for session_start or
// session_shutdown. It is the second adapter behind Hook, next to
// CommandHook; tool and command kinds never reach it.
type ClaudeHook struct {
	name    string
	kind    Kind
	matcher *regexp.Regexp
	command string
	args    []string
	timeout time.Duration
	vars    map[string]string
}

// Name returns "plugin:<Name>/<Event>#<n>".
func (h *ClaudeHook) Name() string { return h.name }

// Match is false: a plugin hook never runs for a tool.
func (*ClaudeHook) Match(string) bool { return false }

// PreTool is a no-op allow.
func (*ClaudeHook) PreTool(context.Context, Event) (PreResult, error) {
	return PreResult{Action: ActionAllow}, nil
}

// PostTool is a no-op.
func (*ClaudeHook) PostTool(context.Context, Event) (PostResult, error) { return PostResult{}, nil }

// Command is a no-op.
func (*ClaudeHook) Command(context.Context, CommandEvent) (CommandResult, error) {
	return CommandResult{}, nil
}

// Session runs the hook when the event is its kind, the reason maps to a
// Claude source, and the matcher accepts that source.
func (h *ClaudeHook) Session(ctx context.Context, ev SessionEvent) (SessionResult, error) {
	allow := SessionResult{Action: ActionAllow}
	if ev.Kind != h.kind {
		return allow, nil
	}
	source, ok := claudeSource(h.kind, ev.Reason)
	if !ok {
		return allow, nil
	}
	if h.matcher != nil && !h.matcher.MatchString(source) {
		return allow, nil
	}
	return h.run(ctx, ev, source)
}

// claudeSource maps a cozyphi reason to SessionStart.source or
// SessionEnd.reason; SessionStart has no answer for an unknown reason.
func claudeSource(kind Kind, reason string) (string, bool) {
	if kind == KindSessionStart {
		switch reason {
		case ReasonStartup:
			return "startup", true
		case ReasonNew:
			return "clear", true
		case ReasonResume:
			return "resume", true
		case ReasonCompact:
			return "compact", true
		}
		return "", false
	}
	switch reason {
	case ReasonNew:
		return "clear", true
	case ReasonResume:
		return "resume", true
	case ReasonQuit:
		return "prompt_input_exit", true
	}
	return "other", true
}

// claudeWireIn is Claude Code's session hook input. transcript_path is left
// out on purpose: cozyphi's session file is not a Claude transcript.
type claudeWireIn struct {
	SessionID     string `json:"session_id"`
	Cwd           string `json:"cwd"`
	HookEventName string `json:"hook_event_name"`
	Source        string `json:"source,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

func (h *ClaudeHook) run(ctx context.Context, ev SessionEvent, source string) (SessionResult, error) {
	in := claudeWireIn{SessionID: ev.SessionID, Cwd: ev.Cwd, HookEventName: "SessionEnd", Reason: source}
	if h.kind == KindSessionStart {
		in = claudeWireIn{SessionID: ev.SessionID, Cwd: ev.Cwd, HookEventName: "SessionStart", Source: source}
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return SessionResult{}, fmt.Errorf("hook %s: encode input: %w", h.name, err)
	}
	if data := h.vars[EnvClaudePluginData]; data != "" {
		if err := os.MkdirAll(data, 0o700); err != nil {
			return SessionResult{}, fmt.Errorf("hook %s: create %s: %w — check its permissions", h.name, data, err)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	cmd := h.newCmd(ctx)
	cmd.Dir = cmp.Or(h.vars[EnvClaudeProjectDir], ev.Cwd)
	cmd.Env = h.env(ev)
	cmd.Stdin = bytes.NewReader(append(payload, '\n'))
	var stdout, stderr limitedBuffer
	stdout.limit, stderr.limit = MaxHookOutputBytes, MaxHookOutputBytes
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// A hook that backgrounds a child leaves the pipes open after it exits;
	// WaitDelay stops waiting for them one second later.
	cmd.WaitDelay = time.Second

	err = cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return SessionResult{}, fmt.Errorf("hook %s timed out after %s — raise its timeout (max %s) or fix the script",
			h.name, h.timeout, maxTimeout)
	}
	if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			return SessionResult{}, fmt.Errorf("hook %s exited %d: %s",
				h.name, ee.ExitCode(), redact.Redact(firstLine(string(stderr.Bytes()))))
		}
		return SessionResult{}, fmt.Errorf("hook %s: %w", h.name, err)
	}
	return claudeResult(stdout.Bytes()), nil
}

// newCmd runs args as exec with placeholders substituted, or the command
// through bash -c. The shell form is not substituted textually: bash expands
// ${CLAUDE_PLUGIN_ROOT} from the environment itself, so a path holding a
// quote or a dollar sign cannot rewrite the command.
func (h *ClaudeHook) newCmd(ctx context.Context) *exec.Cmd {
	if len(h.args) == 0 {
		return exec.CommandContext(ctx, "bash", "-c", h.command) //nolint:gosec // G204: the user enabled this plugin
	}
	expand := varReplacer(h.vars)
	args := make([]string, len(h.args))
	for i, a := range h.args {
		args[i] = expand.Replace(a)
	}
	return exec.CommandContext(ctx, expand.Replace(h.command), args...) //nolint:gosec // G204: the user enabled this plugin
}

func (h *ClaudeHook) env(ev SessionEvent) []string {
	env := sanitizeEnv(environ(), hookEnv{
		Event:      string(h.kind),
		SessionID:  ev.SessionID,
		Cwd:        ev.Cwd,
		ProjectDir: cmp.Or(h.vars[EnvClaudeProjectDir], ev.Cwd),
	})
	for _, k := range slices.Sorted(maps.Keys(h.vars)) {
		env = append(env, k+"="+h.vars[k]) // later duplicates win in os/exec
	}
	return env
}

func varReplacer(vars map[string]string) *strings.Replacer {
	pairs := make([]string, 0, 2*len(vars))
	for _, k := range slices.Sorted(maps.Keys(vars)) {
		pairs = append(pairs, "${"+k+"}", vars[k])
	}
	return strings.NewReplacer(pairs...)
}

type claudeWireOut struct {
	HookSpecificOutput struct {
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
	AdditionalContext      string `json:"additionalContext"`
	AdditionalContextSnake string `json:"additional_context"`
	SystemMessage          string `json:"systemMessage"`
}

// claudeResult reads exit-0 stdout: a JSON object supplies context and a
// toast; any other non-empty text is context verbatim.
func claudeResult(stdout []byte) SessionResult {
	res := SessionResult{Action: ActionAllow}
	text := strings.TrimSpace(string(stdout))
	if text == "" {
		return res
	}
	var out claudeWireOut
	if strings.HasPrefix(text, "{") && json.Unmarshal([]byte(text), &out) == nil {
		res.Context = capContext(cmp.Or(
			out.HookSpecificOutput.AdditionalContext, out.AdditionalContext, out.AdditionalContextSnake))
		res.Toast = out.SystemMessage
		return res
	}
	res.Context = capContext(text)
	return res
}

func capContext(s string) string {
	if len(s) <= MaxPluginContextBytes {
		return s
	}
	cut := MaxPluginContextBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + pluginContextMarker
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}
```

If `limitedBuffer` has no `Bytes()` method, add `func (l *limitedBuffer) Bytes() []byte { return l.buf.Bytes() }` next to `Write` in `command.go`. `CommandHook.spawn` already calls `stdout.Bytes()`, so the method most likely exists. If the package already declares `firstLine`, reuse it and drop the copy above.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins test ./internal/hooks/...`
Expected: PASS, including every existing test, because the variadic parameter leaves current callers unchanged.

- [ ] **Step 6: Format, diagnose, commit**

```bash
make -C /Users/zol/src/cozyphi/.worktrees/claude-plugins fmt
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins add internal/hooks
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins commit -S -m "feat(hooks): run claude code plugin session hooks"
```

---

### Task 3: Plugin discovery

**Files:**
- Create: `internal/plugin/plugin.go`
- Create: `internal/plugin/discover.go`
- Create: `internal/plugin/discover_test.go`
- Modify: `internal/arch/boundaries_test.go` (add one rule next to the `internal/llm` rule at ~202)
- Modify: `internal/tui/controller/developer_mode_coverage_test.go:273` (`envCoverage`: add `COZYPHI_PLUGINS`)

**Interfaces:**
- Consumes: `skills.Source` and `skills.Sources` (Task 1); `hooks.PluginHooks` and the `hooks.EnvClaude*` constants (Task 2); `debuglog.Logf`.
- Produces:
  - `const EnvPlugins = "COZYPHI_PLUGINS"`
  - `func Disabled() bool`
  - `type Config struct { Enabled bool; ClaudeDir string; Paths []string; LocalDataDir string }`
  - `type Plugin struct { ID, Name, Root, DataDir string; SkillDirs, HookFiles []string }`
  - `func (Plugin) Vars(projectRoot string) map[string]string`
  - `func (Plugin) SkillSources(projectRoot string) skills.Sources`
  - `func (Plugin) HookSources(projectRoot string) hooks.PluginHooks`
  - `type Warning struct { Plugin, Msg string }` with `String()`
  - `func Discover(cfg Config, projectRoot string) ([]Plugin, []Warning)`

- [ ] **Step 1: Write the failing tests**

`internal/plugin/discover_test.go`:

```go
package plugin_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/plugin"
)

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	writeFile(t, path, string(raw))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// pluginRoot plants a plugin with one skill and a hooks.json; manifest may be nil.
func pluginRoot(t *testing.T, manifest map[string]any) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: d\n---\n")
	writeFile(t, filepath.Join(root, "hooks", "hooks.json"), `{"hooks":{}}`)
	if manifest != nil {
		writeJSON(t, filepath.Join(root, ".claude-plugin", "plugin.json"), manifest)
	}
	return root
}

type install = map[string]string

func installed(t *testing.T, claudeDir string, plugins map[string][]install) {
	t.Helper()
	writeJSON(t, filepath.Join(claudeDir, "plugins", "installed_plugins.json"),
		map[string]any{"version": 2, "plugins": plugins})
}

func enable(t *testing.T, settingsFile string, enabled map[string]any) {
	t.Helper()
	writeJSON(t, settingsFile, map[string]any{"enabledPlugins": enabled})
}

func discover(t *testing.T, cfg plugin.Config, projectRoot string) ([]plugin.Plugin, []plugin.Warning) {
	t.Helper()
	t.Setenv(plugin.EnvPlugins, "")
	cfg.Enabled = true
	return plugin.Discover(cfg, projectRoot)
}

func TestDiscoverEnabledClaudePlugin(t *testing.T) {
	claude, project := t.TempDir(), t.TempDir()
	root := pluginRoot(t, map[string]any{"name": "ignored-for-installed"})
	const id = "superpowers@claude-plugins-official"
	installed(t, claude, map[string][]install{id: {{"scope": "user", "installPath": root, "version": "6.4.1"}}})
	enable(t, filepath.Join(claude, "settings.json"), map[string]any{id: true})

	got, warns := discover(t, plugin.Config{ClaudeDir: claude}, project)
	require.Empty(t, warns)
	require.Equal(t, []plugin.Plugin{{
		ID:        id,
		Name:      "superpowers",
		Root:      root,
		DataDir:   filepath.Join(claude, "plugins", "data", "superpowers-claude-plugins-official"),
		SkillDirs: []string{filepath.Join(root, "skills")},
		HookFiles: []string{filepath.Join(root, "hooks", "hooks.json")},
	}}, got)
}

func TestDiscoverEnablesOnlyLiteralTrue(t *testing.T) {
	claude := t.TempDir()
	plugins := map[string][]install{}
	enabled := map[string]any{}
	for id, value := range map[string]any{"a@m": "true", "b@m": 1, "c@m": false, "d@m": nil} {
		plugins[id] = []install{{"installPath": pluginRoot(t, nil)}}
		enabled[id] = value
	}
	plugins["e@m"] = []install{{"installPath": pluginRoot(t, nil)}} // no key at all
	installed(t, claude, plugins)
	enable(t, filepath.Join(claude, "settings.json"), enabled)

	got, _ := discover(t, plugin.Config{ClaudeDir: claude}, t.TempDir())
	require.Empty(t, got)
}

func TestDiscoverLayersProjectSettings(t *testing.T) {
	claude, project := t.TempDir(), t.TempDir()
	installed(t, claude, map[string][]install{
		"on@m":  {{"installPath": pluginRoot(t, nil)}},
		"off@m": {{"installPath": pluginRoot(t, nil)}},
	})
	enable(t, filepath.Join(claude, "settings.json"), map[string]any{"on@m": true, "off@m": true})
	enable(t, filepath.Join(project, ".claude", "settings.json"), map[string]any{"on@m": false, "off@m": false})
	enable(t, filepath.Join(project, ".claude", "settings.local.json"), map[string]any{"on@m": true})

	got, warns := discover(t, plugin.Config{ClaudeDir: claude}, project)
	require.Empty(t, warns)
	require.Len(t, got, 1)
	require.Equal(t, "on@m", got[0].ID)
}

func TestDiscoverProjectInstallMatchesThroughSymlink(t *testing.T) {
	claude, project := t.TempDir(), t.TempDir()
	link := filepath.Join(t.TempDir(), "project-link")
	require.NoError(t, os.Symlink(project, link))
	userRoot, projectRoot, otherRoot := pluginRoot(t, nil), pluginRoot(t, nil), pluginRoot(t, nil)
	installed(t, claude, map[string][]install{"p@m": {
		{"scope": "user", "installPath": userRoot},
		{"scope": "project", "installPath": otherRoot, "projectPath": t.TempDir()},
		{"scope": "project", "installPath": projectRoot, "projectPath": link},
	}})
	enable(t, filepath.Join(claude, "settings.json"), map[string]any{"p@m": true})

	got, _ := discover(t, plugin.Config{ClaudeDir: claude}, project)
	require.Len(t, got, 1)
	require.Equal(t, projectRoot, got[0].Root, "this project's install beats the user install")

	got, _ = discover(t, plugin.Config{ClaudeDir: claude}, t.TempDir())
	require.Len(t, got, 1)
	require.Equal(t, userRoot, got[0].Root, "another project's install is skipped")
}

func TestDiscoverRejectsUnknownVersionAndGarbage(t *testing.T) {
	claude := t.TempDir()
	file := filepath.Join(claude, "plugins", "installed_plugins.json")
	writeJSON(t, file, map[string]any{"version": 3, "plugins": map[string]any{}})
	got, warns := discover(t, plugin.Config{ClaudeDir: claude}, t.TempDir())
	require.Empty(t, got)
	require.Len(t, warns, 1)
	require.Contains(t, warns[0].String(), "version 3 is not supported")

	writeFile(t, file, "{")
	got, warns = discover(t, plugin.Config{ClaudeDir: claude}, t.TempDir())
	require.Empty(t, got)
	require.Len(t, warns, 1)
	require.Contains(t, warns[0].String(), "invalid JSON")
}

func TestDiscoverLocalPaths(t *testing.T) {
	named := pluginRoot(t, map[string]any{"name": "mine"})
	unnamed := pluginRoot(t, nil)
	file := filepath.Join(t.TempDir(), "file")
	writeFile(t, file, "")
	dataRoot := t.TempDir()

	got, warns := discover(t, plugin.Config{
		Paths:        []string{named, unnamed, "relative/dir", filepath.Join(dataRoot, "absent"), file},
		LocalDataDir: dataRoot,
	}, t.TempDir())
	require.Len(t, got, 2)
	require.Equal(t, "mine", got[0].ID)
	require.Equal(t, "mine", got[0].Name)
	require.Equal(t, filepath.Join(dataRoot, "mine"), got[0].DataDir)
	require.Equal(t, filepath.Base(unnamed), got[1].Name)
	require.Len(t, warns, 3)
	joined := warnText(warns)
	require.Contains(t, joined, "relative/dir: relative plugin path")
	require.Contains(t, joined, "is not a directory")
}

func TestDiscoverKeepsFirstOfTwoSameNames(t *testing.T) {
	claude := t.TempDir()
	installed(t, claude, map[string][]install{"demo@m": {{"installPath": pluginRoot(t, nil)}}})
	enable(t, filepath.Join(claude, "settings.json"), map[string]any{"demo@m": true})
	local := pluginRoot(t, map[string]any{"name": "demo"})

	got, warns := discover(t, plugin.Config{ClaudeDir: claude, Paths: []string{local}}, t.TempDir())
	require.Len(t, got, 1)
	require.Equal(t, "demo@m", got[0].ID)
	require.Len(t, warns, 1)
	require.Contains(t, warns[0].String(), `plugin name "demo" is already used by demo@m`)
}

func TestDiscoverWarnsOnUnsupportedComponentsAndManifestPaths(t *testing.T) {
	root := pluginRoot(t, map[string]any{
		"name":   "demo",
		"skills": []string{"./extra", "../escape"},
		"hooks":  map[string]any{"SessionStart": []any{}},
	})
	writeFile(t, filepath.Join(root, "extra", "x", "SKILL.md"), "---\nname: x\ndescription: x\n---\n")
	writeFile(t, filepath.Join(root, "commands", "c.md"), "")
	writeFile(t, filepath.Join(root, "agents", "a.md"), "")
	writeFile(t, filepath.Join(root, ".mcp.json"), "{}")

	got, warns := discover(t, plugin.Config{Paths: []string{root}, LocalDataDir: t.TempDir()}, t.TempDir())
	require.Len(t, got, 1)
	require.Equal(t, []string{filepath.Join(root, "skills"), filepath.Join(root, "extra")}, got[0].SkillDirs)
	joined := warnText(warns)
	for _, want := range []string{
		`skills path "../escape" leaves the plugin root`, "inline hooks in plugin.json are not supported",
		"commands/ is not supported", "agents/ is not supported", ".mcp.json is not supported",
	} {
		require.Contains(t, joined, want)
	}
}

func TestDiscoverIsOffWhenDisabled(t *testing.T) {
	cfg := plugin.Config{Paths: []string{pluginRoot(t, nil)}, LocalDataDir: t.TempDir()}
	t.Setenv(plugin.EnvPlugins, "")
	got, _ := plugin.Discover(cfg, t.TempDir()) // Enabled is false
	require.Empty(t, got)

	cfg.Enabled = true
	t.Setenv(plugin.EnvPlugins, "OFF")
	require.True(t, plugin.Disabled())
	got, _ = plugin.Discover(cfg, t.TempDir())
	require.Empty(t, got)
}

func TestPluginSourcesCarryNamespaceAndVars(t *testing.T) {
	p := plugin.Plugin{
		Name: "demo", Root: "/r", DataDir: "/d",
		SkillDirs: []string{"/r/skills"}, HookFiles: []string{"/r/hooks/hooks.json"},
	}
	vars := map[string]string{
		hooks.EnvClaudePluginRoot: "/r", hooks.EnvClaudePluginData: "/d", hooks.EnvClaudeProjectDir: "/proj",
	}
	require.Equal(t, vars, p.Vars("/proj"))
	src := p.SkillSources("/proj")
	require.Len(t, src, 1)
	require.Equal(t, "demo", src[0].Namespace)
	require.Equal(t, vars, src[0].Vars)
	require.Equal(t, hooks.PluginHooks{Name: "demo", Files: p.HookFiles, Vars: vars}, p.HookSources("/proj"))
}

func warnText(warns []plugin.Warning) string {
	out := make([]string, 0, len(warns))
	for _, w := range warns {
		out = append(out, w.String())
	}
	return strings.Join(out, "\n")
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins test ./internal/plugin/...`
Expected: build failure, `no non-test Go files` or `undefined: plugin.Config`.

- [ ] **Step 3: Implement `plugin.go`**

```go
// Package plugin finds the Claude Code plugins cozyphi loads — those enabled
// in Claude Code's own settings and those listed under plugins.paths — and
// describes each one as the skill sources and hook files the rest of cozyphi
// consumes. It reads the disk and nothing else: a plugin is data.
package plugin

import (
	"os"
	"strings"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/llm/skills"
)

// EnvPlugins set to "off" (any case) disables plugin loading for the process.
const EnvPlugins = "COZYPHI_PLUGINS"

// Disabled reports whether COZYPHI_PLUGINS=off.
func Disabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(EnvPlugins)), "off")
}

// Config is the plugins section of config.yaml after defaults and ~ expansion.
type Config struct {
	Enabled bool
	// ClaudeDir is Claude Code's home (~/.claude): installed_plugins.json and
	// settings.json are read from it, never written.
	ClaudeDir string
	// Paths are local plugin roots, absolute, always enabled.
	Paths []string
	// LocalDataDir is the parent of a local plugin's data directory.
	LocalDataDir string
}

// Plugin is one enabled plugin, resolved to absolute paths.
type Plugin struct {
	ID        string   // "superpowers@claude-plugins-official"; a local plugin's Name
	Name      string   // namespace for skills and hook names
	Root      string   // ${CLAUDE_PLUGIN_ROOT}
	DataDir   string   // ${CLAUDE_PLUGIN_DATA}; created by the first hook run
	SkillDirs []string // skills/, a root holding SKILL.md, then manifest extras
	HookFiles []string // hooks/hooks.json, then manifest extras
}

// Warning is a discovery problem that skipped something without failing.
type Warning struct {
	Plugin string // ID, or the path when no ID could be read
	Msg    string // says what is wrong and what to do
}

func (w Warning) String() string {
	if w.Plugin == "" {
		return w.Msg
	}
	return w.Plugin + ": " + w.Msg
}

// Vars are the placeholder values the plugin's skills and hooks see.
func (p Plugin) Vars(projectRoot string) map[string]string {
	return map[string]string{
		hooks.EnvClaudePluginRoot: p.Root,
		hooks.EnvClaudePluginData: p.DataDir,
		hooks.EnvClaudeProjectDir: projectRoot,
	}
}

// SkillSources is one namespaced source per skill directory.
func (p Plugin) SkillSources(projectRoot string) skills.Sources {
	vars := p.Vars(projectRoot)
	out := make(skills.Sources, 0, len(p.SkillDirs))
	for _, dir := range p.SkillDirs {
		out = append(out, skills.Source{Dir: dir, Namespace: p.Name, Vars: vars})
	}
	return out
}

// HookSources describes the plugin's hook files for hooks.Discover.
func (p Plugin) HookSources(projectRoot string) hooks.PluginHooks {
	return hooks.PluginHooks{Name: p.Name, Files: p.HookFiles, Vars: p.Vars(projectRoot)}
}
```

- [ ] **Step 4: Implement `discover.go`**

```go
package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/alvnukov/cozyphi/internal/debuglog"
)

var (
	// validName keeps a plugin name usable as a skill namespace and as a
	// directory name: no ":" (the namespace separator), no path separators,
	// no leading dot.
	validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	// unsafeDataChars mirrors Claude Code's data directory naming, so both
	// harnesses share one directory per plugin.
	unsafeDataChars = regexp.MustCompile(`[^A-Za-z0-9_-]`)
	// unsupported are plugin components cozyphi does not load yet.
	unsupported = []string{"commands/", "agents/", "output-styles/", "monitors/", ".mcp.json", ".lsp.json"}
)

type candidate struct {
	id      string // "" for a local plugin: the manifest names it
	root    string
	dataDir string // "" for a local plugin: derived from the name
}

// Discover returns the enabled plugins: Claude-installed ones sorted by ID,
// then plugins.paths in config order. It never fails: every problem is a
// Warning, and a missing directory means no plugins.
func Discover(cfg Config, projectRoot string) ([]Plugin, []Warning) {
	if !cfg.Enabled || Disabled() {
		return nil, nil
	}
	claude, warns := claudeCandidates(cfg.ClaudeDir, projectRoot)
	local, more := localCandidates(cfg.Paths)
	warns = append(warns, more...)

	var out []Plugin
	owner := make(map[string]string) // Name → ID of the plugin that took it
	for _, c := range append(claude, local...) {
		p, more, ok := build(c, cfg.LocalDataDir)
		warns = append(warns, more...)
		if !ok {
			continue
		}
		if prev, taken := owner[p.Name]; taken {
			warns = append(warns, Warning{Plugin: p.ID, Msg: fmt.Sprintf(
				"plugin name %q is already used by %s; skipped — disable one of them", p.Name, prev)})
			continue
		}
		owner[p.Name] = p.ID
		debuglog.Logf("plugin: loaded %s from %s (%d skill dirs, %d hook files)",
			p.ID, p.Root, len(p.SkillDirs), len(p.HookFiles))
		out = append(out, p)
	}
	for _, w := range warns {
		debuglog.Logf("plugin: %s", w)
	}
	return out, warns
}

type installedFile struct {
	Version int                       `json:"version"`
	Plugins map[string][]installEntry `json:"plugins"`
}

type installEntry struct {
	Scope       string `json:"scope"`
	InstallPath string `json:"installPath"`
	ProjectPath string `json:"projectPath"`
}

func claudeCandidates(claudeDir, projectRoot string) ([]candidate, []Warning) {
	if claudeDir == "" {
		return nil, nil
	}
	file := filepath.Join(claudeDir, "plugins", "installed_plugins.json")
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []Warning{{Plugin: file, Msg: fmt.Sprintf(
			"read: %v — check its permissions or set plugins.claude_dir", err)}}
	}
	var inst installedFile
	if err := json.Unmarshal(data, &inst); err != nil {
		return nil, []Warning{{Plugin: file, Msg: fmt.Sprintf(
			"invalid JSON: %v — reinstall plugins from Claude Code or fix the file", err)}}
	}
	if inst.Version != 2 {
		return nil, []Warning{{Plugin: file, Msg: fmt.Sprintf(
			"version %d is not supported; Claude-installed plugins skipped — cozyphi reads version 2", inst.Version)}}
	}
	enabled, warns := enabledPlugins(claudeDir, projectRoot)
	var out []candidate
	for _, id := range slices.Sorted(maps.Keys(inst.Plugins)) {
		if !enabled[id] {
			continue
		}
		entry, ok := pickInstall(inst.Plugins[id], projectRoot)
		if !ok {
			continue
		}
		if !filepath.IsAbs(entry.InstallPath) {
			warns = append(warns, Warning{Plugin: id, Msg: fmt.Sprintf(
				"installPath %q is not absolute; skipped — reinstall the plugin in Claude Code", entry.InstallPath)})
			continue
		}
		out = append(out, candidate{
			id:      id,
			root:    filepath.Clean(entry.InstallPath),
			dataDir: filepath.Join(claudeDir, "plugins", "data", unsafeDataChars.ReplaceAllString(id, "-")),
		})
	}
	return out, warns
}

// pickInstall prefers the entry installed for this project, then the first
// entry without a projectPath; another project's entry never counts.
func pickInstall(entries []installEntry, projectRoot string) (installEntry, bool) {
	var (
		user  installEntry
		found bool
	)
	for _, e := range entries {
		if e.ProjectPath == "" {
			if !found {
				user, found = e, true
			}
			continue
		}
		if sameDir(e.ProjectPath, projectRoot) {
			return e, true
		}
	}
	return user, found
}

// sameDir compares directories by identity, so a symlinked or differently
// spelled projectPath still names this project.
func sameDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	sa, err := os.Stat(a)
	if err != nil {
		return false
	}
	sb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(sa, sb)
}

// enabledPlugins merges enabledPlugins from the user, project and local
// settings, later files winning per key. Only a literal true enables.
func enabledPlugins(claudeDir, projectRoot string) (map[string]bool, []Warning) {
	files := []string{filepath.Join(claudeDir, "settings.json")}
	if projectRoot != "" {
		files = append(files,
			filepath.Join(projectRoot, ".claude", "settings.json"),
			filepath.Join(projectRoot, ".claude", "settings.local.json"))
	}
	merged := make(map[string]json.RawMessage)
	var warns []Warning
	for _, file := range files {
		data, err := os.ReadFile(file)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			warns = append(warns, Warning{Plugin: file, Msg: fmt.Sprintf("read: %v — check its permissions", err)})
			continue
		}
		var settings struct {
			EnabledPlugins map[string]json.RawMessage `json:"enabledPlugins"`
		}
		if err := json.Unmarshal(data, &settings); err != nil {
			warns = append(warns, Warning{Plugin: file, Msg: fmt.Sprintf(
				"invalid JSON: %v — its enabledPlugins are ignored until the file parses", err)})
			continue
		}
		maps.Copy(merged, settings.EnabledPlugins)
	}
	out := make(map[string]bool, len(merged))
	for id, raw := range merged {
		out[id] = bytes.Equal(bytes.TrimSpace(raw), []byte("true"))
	}
	return out, warns
}

func localCandidates(paths []string) ([]candidate, []Warning) {
	var (
		out   []candidate
		warns []Warning
	)
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			warns = append(warns, Warning{Plugin: p, Msg: "relative plugin path skipped — use an absolute path or ~/…"})
			continue
		}
		st, err := os.Stat(p)
		switch {
		case err != nil:
			warns = append(warns, Warning{Plugin: p, Msg: fmt.Sprintf(
				"plugin path unavailable: %v — fix plugins.paths in config.yaml", err)})
		case !st.IsDir():
			warns = append(warns, Warning{Plugin: p, Msg: "plugin path is not a directory — point plugins.paths at the plugin root"})
		default:
			out = append(out, candidate{root: filepath.Clean(p)})
		}
	}
	return out, warns
}

type manifest struct {
	Name   string          `json:"name"`
	Skills json.RawMessage `json:"skills"`
	Hooks  json.RawMessage `json:"hooks"`
}

func build(c candidate, localDataDir string) (Plugin, []Warning, bool) {
	label := c.id
	if label == "" {
		label = c.root
	}
	var warns []Warning
	warn := func(format string, args ...any) {
		warns = append(warns, Warning{Plugin: label, Msg: fmt.Sprintf(format, args...)})
	}
	if !isDir(c.root) {
		warn("plugin root %s is missing — reinstall the plugin in Claude Code or fix the path", c.root)
		return Plugin{}, warns, false
	}
	m, err := readManifest(c.root)
	if err != nil {
		warn("%v — fix .claude-plugin/plugin.json", err)
		return Plugin{}, warns, false
	}

	name := filepath.Base(c.root)
	switch {
	case c.id != "":
		name, _, _ = strings.Cut(c.id, "@")
	case m.Name != "":
		name = m.Name
	}
	if !validName.MatchString(name) {
		warn("plugin name %q cannot be a namespace; skipped — use letters, digits, '.', '_' or '-'", name)
		return Plugin{}, warns, false
	}
	p := Plugin{ID: c.id, Name: name, Root: c.root, DataDir: c.dataDir}
	if p.ID == "" {
		p.ID = name
		p.DataDir = filepath.Join(localDataDir, name)
	}

	if dir := filepath.Join(c.root, "skills"); isDir(dir) {
		p.SkillDirs = append(p.SkillDirs, dir)
	}
	if isFile(filepath.Join(c.root, "SKILL.md")) {
		p.SkillDirs = append(p.SkillDirs, c.root)
	}
	if file := filepath.Join(c.root, "hooks", "hooks.json"); isFile(file) {
		p.HookFiles = append(p.HookFiles, file)
	}
	p.SkillDirs = addComponents(p.SkillDirs, c.root, "skills", m.Skills, warn)
	p.HookFiles = addComponents(p.HookFiles, c.root, "hooks", m.Hooks, warn)

	for _, comp := range unsupported {
		if exists(filepath.Join(c.root, strings.TrimSuffix(comp, "/"))) {
			warn("%s is not supported by cozyphi; skipped", comp)
		}
	}
	return p, warns, true
}

func readManifest(root string) (manifest, error) {
	file := filepath.Join(root, ".claude-plugin", "plugin.json")
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return manifest{}, nil
	}
	if err != nil {
		return manifest{}, fmt.Errorf("read %s: %w", file, err)
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return manifest{}, fmt.Errorf("invalid %s: %w", file, err)
	}
	return m, nil
}

// addComponents appends a manifest path or path list, confined to root.
func addComponents(dst []string, root, field string, raw json.RawMessage, warn func(string, ...any)) []string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return dst
	}
	var paths []string
	switch raw[0] {
	case '{':
		warn("inline %s in plugin.json are not supported; skipped — move them to %s/", field, field)
		return dst
	case '"':
		var one string
		if err := json.Unmarshal(raw, &one); err != nil {
			warn("manifest %s: %v — use a path or an array of paths", field, err)
			return dst
		}
		paths = []string{one}
	default:
		if err := json.Unmarshal(raw, &paths); err != nil {
			warn("manifest %s: %v — use a path or an array of paths", field, err)
			return dst
		}
	}
	for _, rel := range paths {
		abs, ok := confine(root, rel)
		if !ok {
			warn("manifest %s path %q leaves the plugin root; skipped", field, rel)
			continue
		}
		if !slices.Contains(dst, abs) {
			dst = append(dst, abs)
		}
	}
	return dst
}

func confine(root, rel string) (string, bool) {
	if filepath.IsAbs(rel) {
		return "", false
	}
	abs := filepath.Join(root, rel)
	r, err := filepath.Rel(root, abs)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", false
	}
	return abs, true
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func isFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
```

The test `TestDiscoverWarnsOnUnsupportedComponentsAndManifestPaths` expects the warning `skills path "../escape" leaves the plugin root`. The format `manifest %s path %q leaves…` renders `manifest skills path "../escape" leaves the plugin root`, which contains that substring.

- [ ] **Step 5: Add the arch rule and the env coverage entry**

In `internal/arch/boundaries_test.go`, next to the `internal/llm` rule:

```go
	{
		why: "a plugin is data found on disk; discovery that reaches into the loop " +
			"or the tools it feeds cannot be tested without them",
		from: "internal/plugin",
		deny: []string{"internal/agent", "internal/session", "internal/tools"},
	},
```

Keep the field layout of the neighbouring entries (`why`, `from`, `deny`).

In `internal/tui/controller/developer_mode_coverage_test.go`, in the `envCoverage` map next to `"COZYPHI_SKILL_PATH"` (line 273):

```go
		"COZYPHI_PLUGINS":    {diag.CategoryContext, diag.KeyContextSkills},
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins test ./internal/plugin/... ./internal/arch/... ./internal/tui/controller/ -run 'Discover|Plugin|Boundar|Coverage'`
Expected: PASS.

- [ ] **Step 7: Format, diagnose, commit**

```bash
make -C /Users/zol/src/cozyphi/.worktrees/claude-plugins fmt
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins add internal/plugin internal/arch internal/tui/controller/developer_mode_coverage_test.go
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins commit -S -m "feat(plugin): discover claude code plugins"
```

---

### Task 4: `plugins` config section

**Files:**
- Modify: `internal/project/config.go`:
  - `Config` at 33-57;
  - the defaults in `parseConfigFile` at 373-381, and the decode after line 411;
  - `finalizeConfig` at 326-356;
  - `fileConfig` at 538-549;
  - the template at 255-273.
- Modify: `internal/project/project.go`:
  - `LoadConfig` at ~190;
  - `ClaudeProjectsDir` at 89-91, which gains `ClaudeDir` and `PluginDataDir` beside it.
- Create: `internal/project/plugins_test.go`
- Modify: `internal/tui/controller/developer_mode_coverage_test.go` (`reportedConfig` near line 127)

**Interfaces:**
- Consumes: `plugin.Config`, `plugin.Discover`, `plugin.Plugin.SkillSources` and `plugin.Plugin.HookSources`, `plugin.Warning` (Task 3); `skills.Sources` (Task 1); `hooks.PluginHooks` (Task 2).
- Produces:
  - `Config.Plugins plugin.Config`
  - `Config.Skills skills.Sources`: `skill_path` first, then the plugin sources.
  - `Config.PluginHooks []hooks.PluginHooks`
  - `func (*Config) PluginWarnings() []plugin.Warning`
  - `func (GlobalLayout) ClaudeDir() string`
  - `func (GlobalLayout) PluginDataDir() string`

- [ ] **Step 1: Write the failing tests**

`internal/project/plugins_test.go`:

```go
package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/plugin"
)

// pluginProject starts a project in a fresh HOME whose Claude Code has one
// enabled plugin with a skill and a hooks.json.
func pluginProject(t *testing.T, config string) (*Project, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(plugin.EnvPlugins, "")
	t.Setenv("COZYPHI_SKILL_PATH", "")

	root := filepath.Join(home, ".claude", "plugins", "cache", "demo")
	write := func(path, content string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	write(filepath.Join(root, "skills", "s", "SKILL.md"), "---\nname: s\ndescription: d\n---\n")
	write(filepath.Join(root, "hooks", "hooks.json"), `{"hooks":{}}`)
	inst, err := json.Marshal(map[string]any{"version": 2, "plugins": map[string]any{
		"demo@m": []any{map[string]string{"installPath": root}},
	}})
	require.NoError(t, err)
	write(filepath.Join(home, ".claude", "plugins", "installed_plugins.json"), string(inst))
	write(filepath.Join(home, ".claude", "settings.json"), `{"enabledPlugins":{"demo@m":true}}`)
	write(filepath.Join(home, ".cozyphi", "config.yaml"), config)

	p, err := Discover(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, p.LoadConfig())
	return p, root
}

func TestLoadConfigAddsEnabledPluginSources(t *testing.T) {
	p, root := pluginProject(t, "")
	cfg := p.Config()
	require.True(t, cfg.Plugins.Enabled)
	require.Equal(t, p.Global().ClaudeDir(), cfg.Plugins.ClaudeDir)
	require.Len(t, cfg.Skills, 2)
	require.Equal(t, p.Global().SkillsDir(), cfg.Skills[0].Dir)
	require.Empty(t, cfg.Skills[0].Namespace)
	require.Equal(t, filepath.Join(root, "skills"), cfg.Skills[1].Dir)
	require.Equal(t, "demo", cfg.Skills[1].Namespace)
	require.Len(t, cfg.PluginHooks, 1)
	require.Equal(t, "demo", cfg.PluginHooks[0].Name)
	require.Empty(t, cfg.PluginWarnings())
}

func TestLoadConfigHonoursPluginsDisabled(t *testing.T) {
	p, _ := pluginProject(t, "plugins:\n  enabled: false\n")
	require.Len(t, p.Config().Skills, 1)
	require.Empty(t, p.Config().PluginHooks)
}

func TestLoadConfigHonoursPluginsEnvOff(t *testing.T) {
	t.Setenv(plugin.EnvPlugins, "off")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	p, err := Discover(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, p.LoadConfig())
	require.Len(t, p.Config().Skills, 1)
}

func TestLoadConfigExpandsHomeInPluginPaths(t *testing.T) {
	p, _ := pluginProject(t, "plugins:\n  claude_dir: ~/elsewhere\n  paths:\n    - ~/src/mine\n    - relative\n")
	home := filepath.Dir(p.Global().Root())
	cfg := p.Config()
	require.Equal(t, filepath.Join(home, "elsewhere"), cfg.Plugins.ClaudeDir)
	require.Equal(t, []string{filepath.Join(home, "src", "mine"), "relative"}, cfg.Plugins.Paths)
	require.Equal(t, p.Global().PluginDataDir(), cfg.Plugins.LocalDataDir)
	require.Len(t, cfg.Skills, 1, "the claude_dir moved away, and both paths are skipped")
	require.Len(t, cfg.PluginWarnings(), 2)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins test ./internal/project/ -run 'LoadConfig.*Plugin'`
Expected: build failure, `cfg.Plugins undefined`.

- [ ] **Step 3: Implement the config section**

`internal/project/project.go`: replace `ClaudeProjectsDir` and add two methods:

```go
// ClaudeDir returns Claude Code's home (~/.claude). cozyphi reads its plugin
// records and shares its memory corpus, and never writes anything else there.
func (g GlobalLayout) ClaudeDir() string {
	return filepath.Join(filepath.Dir(g.root), ".claude")
}

// ClaudeProjectsDir returns Claude Code's projects root (~/.claude/projects).
// Every project it has ever opened has a directory there, and every corpus the
// harness knows is one `memory/` inside one of them.
func (g GlobalLayout) ClaudeProjectsDir() string {
	return filepath.Join(g.ClaudeDir(), "projects")
}

// PluginDataDir holds the data directories of plugins listed in
// plugins.paths (~/.cozyphi/plugins/data); Claude-installed plugins share
// Claude Code's own data directory instead.
func (g GlobalLayout) PluginDataDir() string { return filepath.Join(g.root, "plugins", "data") }
```

In `LoadConfig`:

```go
	cfg, err := loadConfig(p.global)
	if err != nil {
		return err
	}
	cfg.discoverPlugins(p.CheckoutRoot())
	p.config.Store(cfg)
	return nil
```

`internal/project/config.go`:
- Add imports for `cmp`, `internal/hooks`, `internal/llm/skills` and `internal/plugin`.
- Add the new fields to `Config`, after `SkillPath`:

```go
	// Plugins is the plugins section with ~ expanded and the Claude Code home
	// defaulted; LoadConfig runs discovery over it.
	Plugins plugin.Config
	// Skills is the skill catalog: skill_path first, then one source per
	// enabled plugin skill directory.
	Skills skills.Sources
	// PluginHooks lists the enabled plugins' hook files for hooks.Discover.
	PluginHooks    []hooks.PluginHooks
	pluginWarnings []plugin.Warning
```

- Add `Plugins: plugin.Config{Enabled: true},` to the `cfg := &Config{...}` literal in `parseConfigFile`.
- Add the field to `fileConfig`:

```go
	Plugins       *pluginsFileConfig       `yaml:"plugins"`
```

- Add the type next to `tasksFileConfig`:

```go
// pluginsFileConfig mirrors the plugins YAML section. It is owner data and
// lives only in the global config.
type pluginsFileConfig struct {
	Enabled   *bool    `yaml:"enabled"`
	ClaudeDir string   `yaml:"claude_dir"`
	Paths     []string `yaml:"paths"`
}
```

- In `parseConfigFile`, after the `raw.SkillPath` block:

```go
	if raw.Plugins != nil {
		if raw.Plugins.Enabled != nil {
			cfg.Plugins.Enabled = *raw.Plugins.Enabled
		}
		cfg.Plugins.ClaudeDir = raw.Plugins.ClaudeDir
		cfg.Plugins.Paths = raw.Plugins.Paths
	}
```

- In `finalizeConfig`, after the `SkillPath` default:

```go
	home := filepath.Dir(global.Root())
	cfg.Plugins.ClaudeDir = expandHome(cmp.Or(cfg.Plugins.ClaudeDir, global.ClaudeDir()), home)
	for i, p := range cfg.Plugins.Paths {
		cfg.Plugins.Paths[i] = expandHome(p, home)
	}
	cfg.Plugins.LocalDataDir = global.PluginDataDir()
	cfg.Skills = skills.Sources{{Dir: cfg.SkillPath}}
```

- Add the functions:

```go
// expandHome resolves a leading ~ against home; any other path is returned as written.
func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return path
}

// discoverPlugins appends each enabled plugin's skill directories to the
// catalog and records its hook files. It runs once per LoadConfig, where the
// project root is known; nothing here can fail the load.
func (c *Config) discoverPlugins(projectRoot string) {
	found, warns := plugin.Discover(c.Plugins, projectRoot)
	c.pluginWarnings = warns
	c.PluginHooks = nil
	for _, p := range found {
		c.Skills = append(c.Skills, p.SkillSources(projectRoot)...)
		if len(p.HookFiles) > 0 {
			c.PluginHooks = append(c.PluginHooks, p.HookSources(projectRoot))
		}
	}
}

// PluginWarnings returns what plugin discovery skipped and why, for /hooks list.
func (c *Config) PluginWarnings() []plugin.Warning {
	if c == nil {
		return nil
	}
	return c.pluginWarnings
}
```

If `expandHome` already exists in the package, reuse it. Before adding a helper, check with `grep -rn 'func expandHome' internal/project`.

- Template (`defaultConfigTemplate`): insert before the "remaining sections" paragraph:

```
# Claude Code plugins: their skills and SessionStart/SessionEnd hooks.
# plugins:
#   enabled: true             # COZYPHI_PLUGINS=off disables them per process
#   claude_dir: ~/.claude     # where Claude Code keeps installed_plugins.json
#   paths:                    # local plugin roots, always enabled
#     - ~/src/my-plugin
#
```

Then change the list to `# The remaining sections (permissions, agents, notifications, opencode,\n# plugins, keybinds) keep their built-in defaults until written here; run`. Keep the line width under 80 characters.

- Coverage (`internal/tui/controller/developer_mode_coverage_test.go`, `reportedConfig`): run the coverage test first. It names every config field it cannot map. Then map each new key to `{diag.CategoryContext, diag.KeyContextSkills}`. The expected keys are:

```go
	"internal/project:fileConfig.plugins":        {diag.CategoryContext, diag.KeyContextSkills},
	"internal/project:pluginsFileConfig.enabled":    {diag.CategoryContext, diag.KeyContextSkills},
	"internal/project:pluginsFileConfig.claude_dir": {diag.CategoryContext, diag.KeyContextSkills},
	"internal/project:pluginsFileConfig.paths":      {diag.CategoryContext, diag.KeyContextSkills},
```

If the test reports the nested keys in a different spelling (for example a bare `.claude_dir`), use exactly the spelling it prints.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins test ./internal/project/... ./internal/tui/controller/ -run 'LoadConfig|Config|Coverage'`
Expected: PASS.

- [ ] **Step 5: Format, diagnose, commit**

```bash
make -C /Users/zol/src/cozyphi/.worktrees/claude-plugins fmt
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins add internal/project internal/tui/controller/developer_mode_coverage_test.go
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins commit -S -m "feat(project): configure plugin discovery"
```

---

### Task 5: Thread skill sources instead of a path

**Files (every current `SkillPath` / `LoadSkills` site):**
- `internal/llm/types.go:111-113`
- `internal/llm/skills/skills.go` (remove `LoadSkills`)
- `internal/llm/skills/skills_test.go:84-180`
- `internal/project/config.go:98-99,117-118`
- `internal/project/project_test.go:166,168,223,703,709`
- `internal/agent/engine.go:80,389,544-556,598,745,1349,1375,1388-1400`
- `internal/agent/engine_runner.go:235,280-297`
- `internal/agent/engine_plan_actions.go:316-323,390`
- `internal/agent/prompt/prompt.go:72,151,156,195-203`
- `internal/agent/prompt/skills-prompt.tmpl`
- `internal/tools/agenttool/agent.go:55-58,84-85,158`
- `internal/tools/agenttool/skills.go:23-71`
- `internal/tui/commands/registry.go:82`, `internal/tui/commands/builtins.go:460,941-958`
- `internal/tui/sessions/view.go:123,197,223,1820-1830`
- `internal/tui/controller/controller.go:1178,1197,1219-1224,1419`
- `cmd/bootstrap.go:200-201,216-217`, `cmd/session_ui.go:51`
- Tests:
  - `internal/agent/edit_eval_test.go:182`
  - `internal/agent/engine_plan_actions_test.go:51-54`
  - `internal/agent/engine_runner_skills_test.go:74,121`
  - `internal/agent/model_facts_test.go:33`
  - `internal/agent/session_test.go:125,131`
  - `internal/agent/prompt/prompt_test.go:201`
  - `internal/llm/request_binding_test.go:38`
  - `internal/tools/agenttool/skills_test.go:35,56`
  - `internal/tui/commands/commands_test.go:39,150`
  - `internal/tui/commands/usage_integration_test.go:101`
  - `internal/tui/sessions/sidebar_test.go:244`, plus every `NewView(` call in `internal/tui/sessions/*_test.go`
- Leave alone: `cmd/config.go:40,489`. This is the config editor's `skill_path` field, not the model's.

**Interfaces:**
- Consumes: `skills.Sources` and the two-value `skills.Find` (Task 1); `Config.Skills` (Task 4).
- Produces:
  - `llm.ModelConfig.Skills skills.Sources` (replaces `SkillPath`)
  - `prompt.Options.Skills skills.Sources`
  - `tools.AgentDeps.Skills func() skills.Sources`
  - `commands.Host.Skills() skills.Sources`
  - `commands.SkillsCommand(sources skills.Sources, add func(string), histories ...*usage.Store)`
  - `sessions.NewView(..., cwd, model string, skillSources skills.Sources, contextWindow int, ...)`
  - `(*sessions.View).Skills() skills.Sources`
  - `engine.skillSources` (unexported)

Partial-load policy, the same at every site: `list, err := sources.Load()`. An error with an empty list fails exactly where a missing path failed before. An error with a non-empty list goes to the debug log and the list is used.

- [ ] **Step 1: Write the failing test for the prompt note**

In `internal/agent/prompt/prompt_test.go`, add:

```go
func TestSkillsBlockMapsClaudeToolsOnlyForPlugins(t *testing.T) {
	user, plugin := t.TempDir(), t.TempDir()
	for _, dir := range []string{user, plugin} {
		path := filepath.Join(dir, "s", "SKILL.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("---\nname: s\ndescription: d\n---\n"), 0o600))
	}

	plain, facts := BuildWithFacts(Options{Skills: skills.Sources{{Dir: user}}})
	require.True(t, facts.SkillDir)
	require.Equal(t, 1, facts.Skills)
	require.NotContains(t, plain, "written for Claude Code")

	mixed, facts := BuildWithFacts(Options{Skills: skills.Sources{{Dir: user}, {Dir: plugin, Namespace: "p"}}})
	require.Equal(t, 2, facts.Skills)
	require.Contains(t, mixed, "### p:s")
	require.Contains(t, mixed, "written for Claude Code")
	require.Contains(t, mixed, "`agent_spawn`, then `agent_wait`")

	_, facts = BuildWithFacts(Options{})
	require.False(t, facts.SkillDir)
}
```

Add imports as needed (`os`, `path/filepath`, `internal/llm/skills`).

- [ ] **Step 2: Run it to verify it fails**

Run: `go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins test ./internal/agent/prompt/ -run SkillsBlockMaps`
Expected: build failure, `unknown field Skills in struct literal of type Options`.

- [ ] **Step 3: Change the model config and the prompt**

`internal/llm/types.go`: replace the `SkillPath` field:

```go
	// Skills is the skill catalog: skill_path first, then the skill
	// directories of enabled Claude Code plugins. Nil means none configured.
	Skills skills.Sources
```

Import `github.com/alvnukov/cozyphi/internal/llm/skills`. `skills` imports nothing from cozyphi, so this creates no cycle.

`internal/agent/prompt/prompt.go`:
- `Options.SkillPath string` becomes `Skills skills.Sources`.
- `facts.SkillDir` becomes `len(opts.Skills) > 0`.
- The call becomes `skillsBlock(opts.Skills)`.
- `skillsData` becomes:

```go
type skillsData struct {
	Catalog string
	// Plugins adds the Claude Code tool-name mapping: plugin skills are
	// written for Claude Code's tools.
	Plugins bool
}
```

```go
func skillsBlock(sources skills.Sources) (string, int) {
	if len(sources) == 0 {
		return "", 0
	}
	list, err := sources.Load()
	if err != nil {
		debuglog.Logf("prompt: load skills from %s: %v", sources, err)
	}
	if len(list) == 0 {
		return "", 0
	}
	catalog := strings.TrimSpace(skills.ToPromptMarkdown(list))
	if catalog == "" {
		return "", 0
	}
	return execTmpl(skillsPrompt, skillsData{Catalog: catalog, Plugins: sources.Namespaced()}), len(list)
}
```

If `prompt` does not already import `debuglog`, check with `go list -deps ./internal/debuglog`. It is a leaf package, so the import is safe. Otherwise drop the log line and keep `_ = err`.

`internal/agent/prompt/skills-prompt.tmpl`: append after `{{.Catalog}}`:

```
{{- if .Plugins}}

## Plugin skills
Skills named `<plugin>:<name>` come from Claude Code plugins and are written for Claude Code. Its tool names map to yours:

| Claude Code | cozyphi |
| --- | --- |
| `Skill` | `read` the SKILL.md at the listed Location |
| `Task` / `Agent` | `agent_spawn`, then `agent_wait` |
| `TodoWrite` | `plan` |
| `Bash`, `Read`, `Write`, `Edit`, `Grep` | `bash`, `read`, `write`, `edit`, `grep` |
| `Glob` | `find` |
| `WebFetch`, `WebSearch` | `web` |

`${CLAUDE_PLUGIN_ROOT}` in a skill is the plugin root: the directory above that skill's `skills/` folder.
{{- end}}
```

- [ ] **Step 4: Change the engine, runner, plan actions and agenttool**

`internal/agent/engine.go`:
- Field at line 80: `skillPath string` becomes `skillSources skills.Sources`.
- Construction at 389: `skillSources: cfg.Skills,`.
- Jobs deps at 544-556:

```go
		// skillSources is read as a snapshot here, under the lock rebindTools
		// holds: the closure runs at spawn time, when no lock protects the
		// field. A model swap rebinds, so the snapshot follows the catalog.
		sources := engine.skillSources
		deps := tools.AgentDeps{
			// ...unchanged fields...
			Skills: func() skills.Sources { return sources },
		}
```

- At 598: `engine.skillSources = cfg.Skills`.
- At 745: `Skills: engine.skillSources,` in `prompt.Options`.
- `skillCatalogNames`:

```go
func (engine *Engine) skillCatalogNames() []string {
	list, err := engine.skillSources.Load()
	if err != nil && len(list) == 0 {
		return nil
	}
	names := make([]string, 0, len(list))
	for _, skill := range list {
		names = append(names, skill.Name)
	}
	return names
}
```

- `pendingSkillsInstruction(sources skills.Sources, names []string)`:

```go
	list, _ := sources.Load() // a partial list still resolves what it holds
	targets := make([]string, 0, len(names))
	for _, name := range names {
		if s, _ := skills.Find(list, name); s != nil && s.SkillFilePath != "" {
			targets = append(targets, s.SkillFilePath)
			continue
		}
		targets = append(targets, name)
	}
```

  This drops the old `if err == nil { … } else { targets = append(targets, names...) }` branch, because an empty list already yields the names. Update its caller at 1349 to pass `engine.skillSources`.

`internal/agent/engine_runner.go`: at 235, `renderJobSkills(model.Skills, meta.Skills)`. Then:

```go
func renderJobSkills(sources skills.Sources, names []string) (string, error) {
	if len(names) == 0 {
		return "", nil
	}
	catalog, err := sources.Load()
	if err != nil && len(catalog) == 0 {
		return "", fmt.Errorf("agent: load skills for job from %s: %w", sources, err)
	}
	// ...the loop from Task 1, with the not-installed message naming %s sources...
```

`internal/agent/engine_plan_actions.go`:
- `queuePlanSkills`: use `catalog, err := engine.skillSources.Load()` and `debuglog.Logf("plan: load skills for preload from %s: %v", engine.skillSources, err)`.
- At 390: `pendingSkillsInstruction(engine.skillSources, missing)`.

`internal/tools/agenttool/agent.go`:

```go
	// Skills is the installed-skill catalog spawn validation resolves
	// `skills` names against; nil or empty means no catalog is installed, so
	// only `skills: []` with a no_skill_reason can pass.
	Skills func() skills.Sources
```

```go
	if deps.Skills == nil {
		deps.Skills = func() skills.Sources { return nil }
	}
```

At line 158: `resolveSpawnSkills(deps.Skills(), in.Skills)`.

`internal/tools/agenttool/skills.go`:
- `resolveSpawnSkills(sources skills.Sources, requested []string)` loads with `sources.Load()`. It fails only when `err != nil && len(catalog) == 0`, with the existing message where `%s` now takes `sources`.
- `unknownSkillError(sources skills.Sources, catalog []*skills.Skill, name string)` prints `sources` wherever it printed `skillPath`.

- [ ] **Step 5: Change the TUI, controller and cmd**

`internal/tui/commands/registry.go:82`: `Skills() skills.Sources`.

`internal/tui/commands/builtins.go`:

```go
		PaletteRoot: func(ctx CommandContext) palette.PaletteCommand {
			sources := hostFn(ctx, Host.Skills)
			add := hostFn(ctx, func(h Host) func(string) { return h.AddSkill })
			return SkillsCommand(sources, add, r.history)
		},
```

```go
func SkillsCommand(sources skills.Sources, add func(name string), histories ...*usage.Store) palette.PaletteCommand {
	// ...
	submenu := skillSubcommands(sources, add, history)
```

```go
func skillSubcommands(sources skills.Sources, add func(name string), history *usage.Store) []palette.PaletteCommand {
	list, _ := sources.Load()
	if len(list) == 0 {
```

`internal/tui/sessions/view.go`:
- The field `skillPath string` becomes `skillSources skills.Sources`.
- The `NewView` parameter `cwd, model, skillPath string,` becomes `cwd, model string, skillSources skills.Sources,`, and it is assigned with `skillSources: skillSources,`.
- The Host methods:

```go
// Skills returns the session's skill catalog sources.
func (e *View) Skills() skills.Sources {
	return e.skillSources
}
```

- In `skillNames`: `list, _ := e.skillSources.Load()`.

`internal/tui/controller/controller.go`: rename `skillPathOrDefault` to `skillsOrDefault` at all four call sites (1178, 1197, 1224, 1419):

```go
// skillsOrDefault fills a catalog model's empty skill catalog from the project
// config, so a provider or opencode pick behaves like a configured one at
// every place it is resolved.
func (c *Controller) skillsOrDefault(cfg llm.ModelConfig) llm.ModelConfig {
	if cfg.Skills == nil && c.proj != nil && c.proj.Config() != nil {
		cfg.Skills = c.proj.Config().Skills
	}
	return cfg
}
```

`internal/project/config.go`, `Model()` and `AllModels()`:

```go
	if m.Skills == nil {
		m.Skills = c.Skills
	}
```

```go
		if all[i].Skills == nil {
			all[i].Skills = c.Skills
		}
```

`cmd/bootstrap.go` (both sites):

```go
		if m.Skills == nil {
			m.Skills = b.Config.Skills
		}
```

`cmd/session_ui.go:51`: `cfg.SkillPath` becomes `cfg.Skills`.

`internal/llm/skills/skills.go`: delete `LoadSkills`. In `skills_test.go`, each `LoadSkills(dir)` becomes `Sources{{Dir: dir}}.Load()`. A test that asserted an error for a file path now expects `ErrorContains(err, "is not a directory")`.

- [ ] **Step 6: Migrate the remaining tests**

Apply these mechanical rules and let the compiler list the sites:
- `SkillPath: dir` becomes `Skills: skills.Sources{{Dir: dir}}`.
- `x.SkillPath = dir` becomes `x.Skills = skills.Sources{{Dir: dir}}`.
- An assertion on `.SkillPath` becomes one on `.Skills` (for example `require.Equal(t, skills.Sources{{Dir: dir}}, cfg.Skills)`).
- `engine.skillPath` becomes `engine.skillSources`.
- `llm/request_binding_test.go:38` becomes `cfg.Skills = skills.Sources{{Dir: "/different/skills"}}`.
- `SkillPath()` in the command test fakes becomes `Skills() skills.Sources`.
- `internal/project/project_test.go:703,709` asserts the default catalog. There, compare `cfg.Skills[0].Dir` with the expected directory.

Every `NewView(` call in the sessions tests passes `""` as the skill path, immediately before the context-window integer. Rewrite them in one pass:

```bash
perl -0pi -e 's/(NewView\((?:[^()]|\([^()]*\))*?)""(,\s*\d+,)/$1nil$2/gs' \
  /Users/zol/src/cozyphi/.worktrees/claude-plugins/internal/tui/sessions/*_test.go
```

Fix any call that passes a non-empty path by hand, for example `sidebar_test.go:244`: `skills.Sources{{Dir: <path>}}`.

- [ ] **Step 7: Build and run the tests**

Run:

```bash
go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins build ./...
go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins vet ./internal/llm/... ./internal/agent/... ./internal/tools/agenttool/... ./internal/tui/... ./internal/project/... ./cmd/...
go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins test ./internal/llm/... ./internal/agent/... ./internal/tools/agenttool/... ./internal/tui/... ./internal/project/... ./cmd/...
```

Expected: the build succeeds and all tests PASS. `grep -rn 'SkillPath\|LoadSkills' internal cmd --include='*.go'` shows only `cmd/config.go` and `project.Config.SkillPath` together with its YAML plumbing.

- [ ] **Step 8: Format, diagnose, commit**

```bash
make -C /Users/zol/src/cozyphi/.worktrees/claude-plugins fmt
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins add -A internal cmd
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins commit -S -m "refactor(skills): thread skill sources instead of a path"
```

---

### Task 6: Load plugin hooks in the controller

**Files:**
- Modify: `internal/tui/controller/controller.go`:
  - `ReloadHooks` at 745-763;
  - `ListHooks` at 766-775;
  - `loadHooksManager` at 1788-1799.
- Create: `internal/tui/controller/plugin_hooks_test.go`

**Interfaces:**
- Consumes: `Config.PluginHooks` and `Config.PluginWarnings()` (Task 4); the variadic `hooks.Discover` and `hooks.LoadObserved` (Task 2).
- Produces: `/hooks list` and `/hooks reload` include plugin hooks and plugin warnings. `pluginHooks(proj)` is an unexported helper.

- [ ] **Step 1: Write the failing test**

`internal/tui/controller/plugin_hooks_test.go`:

```go
package controller

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/project"
)

// pluginWorkspace sets up HOME with Claude Code's records for one enabled
// plugin whose hooks.json holds hooksJSON, and returns the discovered project.
func pluginWorkspace(t *testing.T, hooksJSON string) (*project.Project, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_HOOKS", "")
	t.Setenv("COZYPHI_PLUGINS", "")
	root := filepath.Join(home, ".claude", "plugins", "cache", "demo")
	write := func(path, content string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	write(filepath.Join(root, "hooks", "hooks.json"), hooksJSON)
	write(filepath.Join(root, "commands", "c.md"), "")
	inst, err := json.Marshal(map[string]any{"version": 2, "plugins": map[string]any{
		"demo@m": []any{map[string]string{"installPath": root}},
	}})
	require.NoError(t, err)
	write(filepath.Join(home, ".claude", "plugins", "installed_plugins.json"), string(inst))
	write(filepath.Join(home, ".claude", "settings.json"), `{"enabledPlugins":{"demo@m":true}}`)

	cwd, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())
	return proj, cwd
}

func TestListHooksShowsPluginHooksAndWarnings(t *testing.T) {
	proj, _ := pluginWorkspace(t, `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"true"}]}]}}`)
	c := &Controller{proj: proj}

	found, warns, err := c.ListHooks()
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, "plugin:demo/SessionStart#1", found[0].Manifest.Name)
	require.Equal(t, "plugin:demo", found[0].Source)
	text := make([]string, 0, len(warns))
	for _, w := range warns {
		text = append(text, w.String())
	}
	require.Contains(t, strings.Join(text, "\n"), "demo@m: commands/ is not supported")

	loaded, _, err := c.ReloadHooks()
	require.NoError(t, err)
	require.Equal(t, 1, loaded)
}
```

If a bare `&Controller{proj: proj}` panics in `ReloadHooks` (for example in `storeHooks`), build the controller with `newReadyController`-style setup instead: set the `COZYPHI_MODEL`, `COZYPHI_API_KEY` and `COZYPHI_BASE_URL` env vars, then call `NewController(NewBus(nil), proj, cwd, "")`.

- [ ] **Step 2: Run it to verify it fails**

Run: `go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins test ./internal/tui/controller/ -run ListHooksShowsPlugin`
Expected: FAIL, `found` is empty.

- [ ] **Step 3: Implement**

```go
// pluginHooks returns the enabled plugins' hook files. Plugin discovery ran
// at LoadConfig; /hooks reload re-reads their hooks.json, not the plugin set.
func pluginHooks(proj *project.Project) []hooks.PluginHooks {
	if proj == nil || proj.Config() == nil {
		return nil
	}
	return proj.Config().PluginHooks
}
```

In `ReloadHooks` and `loadHooksManager`:

```go
	mgr, facts, warns, err := hooks.LoadObserved(proj.Global().HooksDir(), proj.HooksDir(), pluginHooks(proj)...)
```

In `ListHooks`:

```go
	found, warns, err := hooks.Discover(proj.Global().HooksDir(), proj.HooksDir(), pluginHooks(proj)...)
	if cfg := proj.Config(); cfg != nil {
		for _, w := range cfg.PluginWarnings() {
			warns = append(warns, hooks.Warning{Path: w.Plugin, Message: w.Msg})
		}
	}
	return found, warns, err
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins test ./internal/tui/controller/ -run 'Hook'`
Expected: PASS.

- [ ] **Step 5: Format, diagnose, commit**

```bash
make -C /Users/zol/src/cozyphi/.worktrees/claude-plugins fmt
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins add internal/tui/controller
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins commit -S -m "feat(tui): load claude plugin hooks with hook manifests"
```

---

### Task 7: Deliver session context once

**Files:**
- Create: `internal/agent/session_context.go`
- Create: `internal/agent/session_context_test.go`
- Modify: `internal/agent/engine.go`:
  - add `EngineOpts.LifecycleHooks` at ~303;
  - add the fields next to `compactAdvice` at 154;
  - update the construction at ~415;
  - change the executor wiring at 819;
  - update `composeUserPrompt` at ~1362.
- Modify: `internal/agent/executor.go` (`drainCompactAdvice` field at 80-85; `SetCompactAdviceDrain` at 171-180; the drain at 526)
- Modify: `internal/tui/controller/controller.go`:
  - `newEngine` at 301 (`LifecycleHooks: true`);
  - `emitSessionStart` at 2931 and its callers at 294 and 2285.
- Create: `internal/tui/controller/plugin_context_test.go`

**Interfaces:**
- Consumes: `hooks.SessionOutcome.Context` (Task 2); `Config.PluginHooks` (Task 4, through Task 6's controller wiring).
- Produces:
  - `EngineOpts.LifecycleHooks bool`: true only for controller-owned primary engines.
  - `func (*Engine) QueueSessionContext(text string)`: replace semantics, delivered once.
  - `func (*Engine) drainSessionContext() string`
  - `func (*Engine) drainBoundaryReminders() string`
  - `func (*Executor) SetReminderDrain(drain func() string)` (renamed from `SetCompactAdviceDrain`)
  - `func (c *Controller) emitSessionStart(eng *agent.Engine, reason, previousID string)`

- [ ] **Step 1: Write the failing engine tests**

`internal/agent/session_context_test.go`:

```go
package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func lifecycleEngine(t *testing.T, url string, parentID string) *Engine {
	t.Helper()
	engine, err := NewEngine(EngineOpts{
		Model:          llm.ModelConfig{Name: "fake", BaseURL: url, APIKey: "x", ContextWindow: 100000},
		SessionOpts:    SessionOpts{Cwd: t.TempDir(), ParentID: parentID},
		LifecycleHooks: true,
	})
	require.NoError(t, err)
	return engine
}

func TestQueuedSessionContextIsDeliveredOnce(t *testing.T) {
	server, bodies := capturingTextServer(t)
	engine := lifecycleEngine(t, server.URL, "")

	engine.QueueSessionContext("FIRST-BOOT-SENTINEL")
	engine.QueueSessionContext("SECOND-BOOT-SENTINEL")
	drainLoop(t, engine, "hello")
	drainLoop(t, engine, "again")

	got := bodies()
	require.Len(t, got, 2)
	require.NotContains(t, got[0], "FIRST-BOOT-SENTINEL", "a later queue replaces the earlier one")
	require.Equal(t, 1, strings.Count(got[0], "SECOND-BOOT-SENTINEL"))
	require.Contains(t, got[0], "<system-reminder>")
	require.Equal(t, 1, strings.Count(got[1], "SECOND-BOOT-SENTINEL"), "history carries it; nothing re-injects it")
}

func TestSessionContextRidesTheFirstToolResult(t *testing.T) {
	server, streams, bodies := fakeContextServer(t, "unused", func(n int32) string {
		if n == 1 {
			return sseToolCallChunk("call_1", "context", `{}`)
		}
		return sseTextChunk()
	})
	engine := lifecycleEngine(t, server.URL, "")

	var queued bool
	for _, err := range engine.Loop(t.Context(), "hello", LoopOpts{}) {
		require.NoError(t, err)
		// The first request is on the wire, so the prompt is already composed;
		// Loop is a pull iterator, so the tool has not run yet.
		if !queued && streams.Load() >= 1 {
			engine.QueueSessionContext("MID-TURN-SENTINEL")
			queued = true
		}
	}
	require.True(t, queued)
	all := bodies()
	require.GreaterOrEqual(t, len(all), 2)
	require.NotContains(t, all[0], "MID-TURN-SENTINEL")
	require.Equal(t, 1, strings.Count(all[1], "MID-TURN-SENTINEL"), "the tool result boundary delivers it")
}

func TestChildEngineIgnoresSessionContext(t *testing.T) {
	server, bodies := capturingTextServer(t)
	engine := lifecycleEngine(t, server.URL, "parent-session")

	engine.QueueSessionContext("CHILD-SENTINEL")
	drainLoop(t, engine, "hello")
	require.NotContains(t, bodies()[0], "CHILD-SENTINEL")
}

func TestEngineWithoutLifecycleIgnoresSessionContext(t *testing.T) {
	server, bodies := capturingTextServer(t)
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)

	engine.QueueSessionContext("OFF-SENTINEL")
	drainLoop(t, engine, "hello")
	require.NotContains(t, bodies()[0], "OFF-SENTINEL")
}
```

`sseToolCallChunk` and `sseTextChunk` already exist in the agent tests; check with `grep -n 'func sseToolCallChunk\|func sseTextChunk' internal/agent/*_test.go`. If a tool call to `context` with `{}` does not run cleanly under this setup, use the tool call that `TestLoopContextToolStatusReachesModel` uses, since that test already drives this exact server shape.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins test ./internal/agent/ -run 'SessionContext|Lifecycle'`
Expected: build failure, `unknown field LifecycleHooks`.

- [ ] **Step 3: Implement the queue**

`internal/agent/engine.go`:
- `EngineOpts` gains:

```go
	// LifecycleHooks marks a controller-owned primary engine: it accepts
	// session-start context and re-runs session_start hooks after
	// compaction. Children and headless runs leave it off.
	LifecycleHooks bool
```

- The engine struct, next to `compactAdvice`, gains:

```go
	// lifecycle is LifecycleHooks for a primary engine; a child never gets it.
	lifecycle bool
	// sessionContext parks session_start hook context until the next prompt
	// or tool result, whichever comes first. A newer queue replaces it.
	sessionContext string
```

- The construction literal gains `lifecycle: opts.LifecycleHooks && opts.SessionOpts.ParentID == "",`.
- Line 819 becomes `engine.executor.SetReminderDrain(engine.drainBoundaryReminders)`.
- In `composeUserPrompt`, after the memory-recall `prependReminder` and before the compact-advice return:

```go
	// Session context (a plugin bootstrap) precedes the request it frames;
	// compact advice stays outermost.
	content = prependReminder(engine.drainSessionContext(), content)
	return prependReminder(engine.drainCompactAdvice(), content)
```

`internal/agent/session_context.go`:

```go
package agent

import "strings"

// QueueSessionContext parks text a session_start hook returned. It reaches
// the model once, wrapped as a system reminder, at the next composed prompt
// or tool result. A second call before delivery replaces the first: the
// latest session start describes the session that is actually open. An
// engine without lifecycle hooks (a child, a headless run) ignores it.
func (engine *Engine) QueueSessionContext(text string) {
	if engine == nil {
		return
	}
	text = strings.TrimSpace(text)
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if !engine.lifecycle {
		return
	}
	if text == "" {
		engine.sessionContext = ""
		return
	}
	engine.sessionContext = reminderOpen + "\n" + text + "\n" + reminderClose
}

// drainSessionContext takes the parked session context, if any.
func (engine *Engine) drainSessionContext() string {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	reminder := engine.sessionContext
	engine.sessionContext = ""
	return reminder
}

// drainBoundaryReminders is what the executor attaches to a tool result:
// session context first, then compaction advice.
func (engine *Engine) drainBoundaryReminders() string {
	return prependReminder(engine.drainSessionContext(), engine.drainCompactAdvice())
}
```

`internal/agent/executor.go`: rename the field to `drainReminders` and its setter to `SetReminderDrain`, and update its comment:

```go
	// drainReminders, when wired, moves reminders parked on the engine —
	// session context and compaction advice — onto this call's result, one
	// boundary earlier than the next-prompt drain.
	drainReminders func() string
```

```go
// SetReminderDrain wires the engine's parked-reminder drain.
func (e *Executor) SetReminderDrain(drain func() string) {
```

At line 526, rename the uses to `e.drainReminders`. Update every other caller of `SetCompactAdviceDrain` that `grep -rn SetCompactAdviceDrain internal` finds, tests included.

- [ ] **Step 4: Wire the controller**

`internal/tui/controller/controller.go`:
- In `newEngine`, add `LifecycleHooks: true,` to the `agent.EngineOpts` literal.
- `emitSessionStart` gains the engine the context belongs to:

```go
func (c *Controller) emitSessionStart(eng *agent.Engine, reason, previousID string) {
	mgr := c.Hooks()
	if mgr == nil {
		return
	}
	ctx := context.Background()
	if c.runtime != nil {
		ctx = c.runtime.constructionCtx
	}
	out := mgr.SessionStart(ctx, hooks.SessionEvent{
		SessionID:         eng.SessionID(),
		Cwd:               c.cwd,
		Reason:            reason,
		PreviousSessionID: previousID,
		Usage:             c.sessionUsage(),
	})
	c.publishSessionEffects(out)
	eng.QueueSessionContext(out.Context)
}
```

- The callers become `c.emitSessionStart(eng, hooks.ReasonStartup, "")` at 294 and `c.emitSessionStart(eng, reason, prevID)` at 2285.
- Check with happ `code op='calls'` that nothing else calls `emitSessionStart`.

- [ ] **Step 5: Write the controller integration test**

`internal/tui/controller/plugin_context_test.go`:

```go
package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPluginSessionStartContextReachesTheModelOnce(t *testing.T) {
	var (
		mu     sync.Mutex
		bodies []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(raw))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w,
			"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	proj, cwd := pluginWorkspace(t, `{"hooks":{"SessionStart":[{"matcher":"startup|clear|compact",`+
		`"hooks":[{"type":"command","command":"echo PLUGIN-BOOT-SENTINEL"}]}]}}`)
	t.Setenv("COZYPHI_MODEL", "fake")
	t.Setenv("COZYPHI_API_KEY", "x")
	t.Setenv("COZYPHI_BASE_URL", server.URL)
	require.NoError(t, proj.LoadConfig())

	c, err := NewController(NewBus(nil), proj, cwd, "")
	require.NoError(t, err)
	t.Cleanup(c.Close)

	requests := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), bodies...)
	}
	for i, prompt := range []string{"first", "second"} {
		c.StartPrompt(prompt, nil)
		require.Eventually(t, func() bool { return len(requests()) == i+1 && !c.RunActive() },
			5*time.Second, 10*time.Millisecond)
	}
	got := requests()
	require.Equal(t, 1, strings.Count(got[0], "PLUGIN-BOOT-SENTINEL"))
	require.Equal(t, 1, strings.Count(got[1], "PLUGIN-BOOT-SENTINEL"), "delivered once, then only history")
}
```

`pluginWorkspace` is the helper from Task 6 (`plugin_hooks_test.go`). If `c.Close` has a different signature, wrap it in `func() { c.Close() }`.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins test ./internal/agent/... ./internal/tui/controller/...`
Expected: PASS.

- [ ] **Step 7: Format, diagnose, commit**

```bash
make -C /Users/zol/src/cozyphi/.worktrees/claude-plugins fmt
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins add internal/agent internal/tui/controller
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins commit -S -m "feat(agent): deliver plugin session context once"
```

---

### Task 8: Re-run session start after compaction

**Files:**
- Modify: `internal/agent/engine_compaction.go:94-100` (after `forgetDeliveredPlanSkills`)
- Modify: `internal/agent/session_context.go` (add `refireSessionStart`)
- Create: `internal/agent/session_context_compact_test.go`

**Interfaces:**
- Consumes: `hooks.ReasonCompact` and `SessionOutcome.Context` (Task 2); `engine.lifecycle` and `QueueSessionContext` (Task 7); `engine.hooks` (guarded by `engine.mu`); `engine.SessionID()` and `engine.SessionCwd()`.
- Produces: after every successful compaction (overflow recovery and `/compact` alike), a lifecycle engine runs `session_start` with reason `compact` and queues the returned context.

- [ ] **Step 1: Write the failing tests**

`internal/agent/session_context_compact_test.go`:

```go
package agent

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

func compactingEngine(t *testing.T, lifecycle bool, parentID string) (*Engine, *atomic.Int32, func() []string) {
	t.Helper()
	server, _, bodies := fakeContextServer(t, "SUMMARY", func(int32) string { return sseTextChunk() })
	engine, err := NewEngine(EngineOpts{
		Model:          llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x", ContextWindow: 100000},
		SessionOpts:    SessionOpts{Cwd: t.TempDir(), ParentID: parentID},
		LifecycleHooks: lifecycle,
	})
	require.NoError(t, err)
	var calls atomic.Int32
	engine.SetHooks(hooks.NewManager(hooks.Entry{
		Kind: hooks.KindSessionStart,
		Hook: hooks.FuncHook{
			HookName: "boot",
			Sess: func(_ context.Context, ev hooks.SessionEvent) (hooks.SessionResult, error) {
				calls.Add(1)
				require.Equal(t, hooks.ReasonCompact, ev.Reason)
				require.Equal(t, engine.SessionID(), ev.SessionID)
				return hooks.SessionResult{Context: "COMPACT-BOOT-SENTINEL"}, nil
			},
		},
	}))
	seedTwoTurnHistory(t, engine)
	return engine, &calls, bodies
}

func TestCompactionRefiresSessionStartAndDeliversOnce(t *testing.T) {
	engine, calls, bodies := compactingEngine(t, true, "")
	require.NoError(t, engine.CompactNow(t.Context(), func(session.Event) bool { return true }))
	require.EqualValues(t, 1, calls.Load())

	drainLoop(t, engine, "after compact")
	drainLoop(t, engine, "and again")
	all := bodies()
	require.Equal(t, 1, strings.Count(all[len(all)-2], "COMPACT-BOOT-SENTINEL"))
	require.Equal(t, 1, strings.Count(all[len(all)-1], "COMPACT-BOOT-SENTINEL"))
}

func TestCompactionWithoutLifecycleRunsNoHook(t *testing.T) {
	engine, calls, _ := compactingEngine(t, false, "")
	require.NoError(t, engine.CompactNow(t.Context(), func(session.Event) bool { return true }))
	require.Zero(t, calls.Load())
}

func TestChildCompactionRunsNoHook(t *testing.T) {
	engine, calls, _ := compactingEngine(t, true, "parent-session")
	require.NoError(t, engine.CompactNow(t.Context(), func(session.Event) bool { return true }))
	require.Zero(t, calls.Load())
}
```

If `SetHooks` on an engine with `ParentID` is refused, or `seedTwoTurnHistory` does not leave enough to compact in a child session, keep the child case only as `require.Zero(calls)` after `CompactNow`. The error from `CompactNow` does not matter for that case: call `_ = engine.CompactNow(...)` there.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins test ./internal/agent/ -run 'Compaction.*(Refires|Lifecycle|Child)|ChildCompaction'`
Expected: FAIL, `calls` is 0 in the first test.

- [ ] **Step 3: Implement**

In `internal/agent/session_context.go`, add the function and imports for `context`, `hooks` and `debuglog`:

```go
// refireSessionStart runs session_start with reason compact after a
// successful compaction, so a plugin bootstrap summarized away is delivered
// again. The engine has no UI channel: toast and status are logged only.
func (engine *Engine) refireSessionStart(ctx context.Context) {
	engine.mu.Lock()
	mgr, lifecycle := engine.hooks, engine.lifecycle
	engine.mu.Unlock()
	if !lifecycle || mgr == nil {
		return
	}
	out := mgr.SessionStart(ctx, hooks.SessionEvent{
		SessionID: engine.SessionID(),
		Cwd:       engine.SessionCwd(),
		Reason:    hooks.ReasonCompact,
	})
	if out.Toast != "" || out.StatusSet {
		debuglog.Logf("hooks: compact session_start toast=%q status=%q (not shown: no UI here)", out.Toast, out.Status)
	}
	engine.QueueSessionContext(out.Context)
}
```

In `internal/agent/engine_compaction.go`, after `engine.forgetDeliveredPlanSkills()`:

```go
	engine.refireSessionStart(ctx)
```

`ctx` is the context parameter of the compaction function. Check with happ `code op='definition'` on the enclosing function that the parameter is called `ctx`. Also check that `SessionID()` and `SessionCwd()` do not take `engine.mu`; if either does, read them before taking the lock above.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins test ./internal/agent/...`
Expected: PASS.

- [ ] **Step 5: Format, diagnose, commit**

```bash
make -C /Users/zol/src/cozyphi/.worktrees/claude-plugins fmt
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins add internal/agent
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins commit -S -m "feat(agent): re-run session start hooks after compaction"
```

---

### Task 9: Docs, changelog and verification

**Files:**
- Modify: `doc/plugins.md`: mark it implemented and record the clarifications below.
- Modify: `doc/hooks.md`: add a "Claude Code plugin hooks" section and the `compact` reason. Existing `plugin.json` files are now called "hook manifests".
- Modify: `doc/project-layout.md`: add `internal/plugin` and `internal/hooks/claude.go`.
- Modify: `internal/llm/skills/doc.go`: document `Sources`, namespaces and symlink handling.
- Modify: `CHANGELOG.md` under `## [Unreleased]`.
- Modify: `obsidian-tasks/claude-plugins.md`: set status to done once verified.

**Interfaces:**
- Consumes: everything above.
- Produces: documentation only.

- [ ] **Step 1: Update `doc/plugins.md`**

Set the status line to `Status: implemented (2026-09-24). Task: obsidian-tasks/claude-plugins.md.`. Replace the `ParseClaudeHooks` signature block with the variadic `hooks.Discover(userDir, projectDir string, plugins ...PluginHooks)` and `type PluginHooks`. Then add a "Decisions made during implementation" section with these bullets:

- Plugin discovery runs in `project.LoadConfig`, not `cmd`, which yields `Config.Skills` and `Config.PluginHooks`. `/hooks reload` re-reads `hooks.json` but not the plugin set, which a restart refreshes.
- Among install entries, one whose `projectPath` is the project root wins. Otherwise the first entry without `projectPath` wins, and other projects' entries never count. A plugin name must match `^[A-Za-z0-9][A-Za-z0-9._-]*$`, and manifest paths stay inside the plugin root.
- `skills.Sources.Load` returns the partial list together with the joined error. Callers fail only when nothing loaded.
- Context is accepted from `hookSpecificOutput.additionalContext`, `additionalContext` or `additional_context`. `QueueSessionContext` replaces rather than appends.
- Only engines built with `EngineOpts.LifecycleHooks` (controller primaries) accept context or re-fire on compaction. A child controller still runs the hook, but its engine ignores the result.
- In the shell form, placeholders reach the command through the environment (bash expands them). Only `args` are substituted textually, which keeps a path with quotes from rewriting the command.
- The first line of stderr is redacted before it reaches the debug log. Runtime hook errors go to the debug log only. `exec.ErrWaitDelay` after a clean exit counts as success.
- The 16 KiB plugin context cap is separate from the 4 KiB `MaxContextBytes` cap of hook manifests.
- `session_start` runs synchronously, bounded by the hook timeout (30 s by default, 60 s max). Headless runs get plugin skills but no session hooks.
- The harness diag view drops the `plugin:` origin, and the config coverage maps plugin keys to the skills key.
- The executor setter is renamed to `SetReminderDrain`, because it now carries session context as well.
- Skill bodies are expanded at load. The file the model `read`s stays raw.

- [ ] **Step 2: Update `doc/hooks.md`, `doc/project-layout.md` and `skills/doc.go`**

`doc/hooks.md`:
- The reason list becomes `startup | new | resume | compact | quit`. Explain that `compact` fires `session_start` after every successful compaction, in the primary engine only.
- Add a section "Claude Code plugin hooks" with these points:
  - `SessionStart` and `SessionEnd` only;
  - the reason → source table from `doc/plugins.md`;
  - the anchored matcher;
  - `bash -c` versus `args`;
  - the 30 s default and 60 s cap;
  - the stdin fields;
  - the output parsing;
  - failures never block;
  - names are `plugin:<Name>/<Event>#<n>`.
- Rename "plugin" to "hook manifest" wherever the text means `~/.cozyphi/hooks/**/plugin.json`. The file name itself stays `plugin.json`.

`doc/project-layout.md`: add `internal/plugin/`, described as "discovers Claude Code plugins (installed_plugins.json, enabledPlugins, plugins.paths) into skill sources and hook files". Add `internal/hooks/claude.go` under hooks.

`internal/llm/skills/doc.go`: extend the package comment with one paragraph covering four things. A catalog is `Sources`. A plugin source namespaces its skills as `<plugin>:<name>`. Directory symlinks are followed, with cycles guarded by real path. `Find` resolves exact, then case-insensitive, then an unambiguous bare name.

- [ ] **Step 3: CHANGELOG**

Under `## [Unreleased]`, add to the matching subsections (create `### Added` or `### Changed` if missing):

```markdown
### Added
- Claude Code plugins: skills and `SessionStart`/`SessionEnd` hooks of plugins enabled in Claude Code (`~/.claude`) or listed under `plugins.paths` in config.yaml load automatically; plugin skills are named `<plugin>:<skill>`. `COZYPHI_PLUGINS=off` disables them.

### Changed
- `session_start` hooks also run with reason `compact` after every successful compaction, so bootstrap context survives it.
```

- [ ] **Step 4: Verify**

Run the scoped tests:

```bash
go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins test ./internal/plugin/... ./internal/llm/... ./internal/hooks/... ./internal/agent/... ./internal/project/... ./internal/tools/agenttool/... ./internal/tui/... ./internal/arch/... ./cmd/...
```

Expected: PASS.

Run the single scoped lint:

```bash
make -C /Users/zol/src/cozyphi/.worktrees/claude-plugins fmt-check
golangci-lint run --path-prefix /Users/zol/src/cozyphi/.worktrees/claude-plugins \
  /Users/zol/src/cozyphi/.worktrees/claude-plugins/internal/plugin/... \
  /Users/zol/src/cozyphi/.worktrees/claude-plugins/internal/llm/... \
  /Users/zol/src/cozyphi/.worktrees/claude-plugins/internal/hooks/... \
  /Users/zol/src/cozyphi/.worktrees/claude-plugins/internal/agent/... \
  /Users/zol/src/cozyphi/.worktrees/claude-plugins/internal/project/... \
  /Users/zol/src/cozyphi/.worktrees/claude-plugins/internal/tools/agenttool/... \
  /Users/zol/src/cozyphi/.worktrees/claude-plugins/internal/tui/... \
  /Users/zol/src/cozyphi/.worktrees/claude-plugins/cmd/...
```

Expected: no issues. If `golangci-lint` refuses absolute package paths, look at the lint target in the Makefile and run the same scoped command it uses, with these packages passed in.

Manual TUI check, with superpowers installed in Claude Code:
- `go -C /Users/zol/src/cozyphi/.worktrees/claude-plugins run ./cmd/cozyphi` starts.
- `/hooks list` shows `plugin:superpowers/SessionStart#1` with source `plugin:superpowers`.
- `/skills` lists the 15 `superpowers:*` skills.
- The first turn's request carries the `using-superpowers` bootstrap exactly once. Check it with `COZYPHI_DEBUG` or the session file.
- `/compact` is followed by one more delivery of the bootstrap.

- [ ] **Step 5: Close the task and commit**

In `obsidian-tasks/claude-plugins.md`, set `status: done` and `updated_at` to the current UTC time. Keep bold labels in the body; no `## ` headings (see memory `task-body-no-headings`).

```bash
make -C /Users/zol/src/cozyphi/.worktrees/claude-plugins fmt
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins add doc CHANGELOG.md internal/llm/skills/doc.go obsidian-tasks/claude-plugins.md
git -C /Users/zol/src/cozyphi/.worktrees/claude-plugins commit -S -m "docs(plugins): document claude code plugin support"
```

Do not push. Publishing and the PR wait for the user's explicit go-ahead.
