package tasktool_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tasks"
	"github.com/alvnukov/cozyphi/internal/tools/tasktool"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

type fixture struct {
	reg     *tasks.Registry
	targets *tasks.Targets
	root    string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	reg := tasks.Open(root, filepath.Join(root, tasks.DefaultDir))
	for _, d := range []tasks.Draft{
		{ID: "epic-auth", Title: "Auth overhaul", Type: "epic", Priority: "high"},
		{
			ID: "fix-login", Title: "Fix login timeout", Type: "bug", Priority: "critical", ParentID: "epic-auth",
			Body: "Sessions drop after 30s.", AcceptanceCriteria: []string{"idle 5 minutes survives"},
			VerificationPlan: []string{"go test ./internal/auth/..."}, Tags: []string{"auth"},
		},
		{ID: "docs", Title: "Write the docs", Type: "chore", Priority: "low"},
		{ID: "old", Title: "Shipped already", Status: tasks.StatusDone},
		{ID: "stuck", Title: "Waiting on infra", Status: tasks.StatusBlocked, Priority: "high"},
	} {
		_, err := reg.Create(d)
		require.NoError(t, err)
	}
	targets, err := tasks.DiscoverTargets(root, root, nil)
	require.NoError(t, err)
	require.NotNil(t, targets)
	return fixture{reg: reg, targets: targets, root: root}
}

// newTwoRootFixture adds a second, external registry the session may address
// as "other" — the shape a main-plus-worktree or configured-root session has.
func newTwoRootFixture(t *testing.T) (fixture, string) {
	t.Helper()
	f := newFixture(t)
	other := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(other, tasks.DefaultDir), 0o755))
	targets, err := tasks.DiscoverTargets(f.root, f.root, map[string]string{"other": other})
	require.NoError(t, err)
	require.NotNil(t, targets)
	f.targets = targets
	return f, other
}

func (f fixture) ctx(t *testing.T) context.Context {
	t.Helper()
	return tooldef.WithCwd(t.Context(), f.root)
}

// resolved is a path in the spelling targets canonicalize to (symlinks
// resolved), which on some machines differs from the one t.TempDir returns.
func resolved(t *testing.T, path string) string {
	t.Helper()
	out, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return out
}

func (f fixture) run(t *testing.T, args string) (string, string) {
	t.Helper()
	tool := tasktool.Tool(f.targets, tasks.AccessWrite)
	require.Equal(t, "task", tool.Definition.Name)
	res, err := tool.Run(tooldef.WithCwd(t.Context(), resolved(t, f.root)), json.RawMessage(args))
	require.NoError(t, err)
	return res.Content, res.Detail
}

func (f fixture) fail(t *testing.T, args string) error {
	t.Helper()
	_, err := tasktool.Tool(f.targets, tasks.AccessWrite).Run(f.ctx(t), json.RawMessage(args))
	require.Error(t, err)
	return err
}

func TestCurrentRanksOpenWorkAndEndsWithTheNextMove(t *testing.T) {
	f := newFixture(t)
	content, detail := f.run(t, `{}`)

	assert.Equal(t, "current (2 ready)", detail)
	assert.Contains(
		t,
		content,
		"Ready (2), best first:\n1. fix-login · todo · critical · bug — Fix login timeout\n2. docs · todo · low · chore — Write the docs\n",
	)
	assert.Contains(t, content, "Blocked (1):\n- stuck · blocked · high · task — Waiting on infra\n")
	assert.NotContains(t, content, "epic-auth", "a container is not work")
	assert.NotContains(t, content, "old", "done tasks are history")
	assert.True(
		t,
		strings.HasSuffix(content, "Next: task get <id> for the full note, then task start root=main <id> to take it."),
		content,
	)
}

func TestCurrentOnAnEmptyRegistryPointsAtCreate(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, tasks.DefaultDir), 0o755))
	targets, err := tasks.DiscoverTargets(root, root, nil)
	require.NoError(t, err)
	require.NotNil(t, targets)
	f := fixture{reg: tasks.Open(root, filepath.Join(root, "obsidian-tasks")), targets: targets, root: root}
	content, _ := f.run(t, `{"action":"current"}`)
	assert.Contains(t, content, "No open tasks.")
	assert.Contains(t, content, "Next: task create root=main")
}

func TestCurrentReportsNotesItCouldNotRead(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(f.reg.Dir(), "broken.md"), []byte("---\nid: broken\n---\n"), 0o600))
	content, _ := f.run(t, `{}`)
	assert.Contains(
		t,
		content,
		"1 note(s) could not be read and are not listed:\n- broken.md: frontmatter: title is required\n",
	)
}

func TestGetRendersTheWholeNote(t *testing.T) {
	f := newFixture(t)
	content, detail := f.run(t, `{"action":"get","id":"fix-login"}`)

	assert.Equal(t, "get fix-login", detail)
	assert.True(
		t,
		strings.HasPrefix(
			content,
			"fix-login — Fix login timeout\nstatus: todo · priority: critical · model_level: - · type: bug\nparent: epic-auth · tags: auth\nfile: obsidian-tasks/fix-login.md · updated ",
		),
		content,
	)
	assert.Contains(t, content, "\n\nSessions drop after 30s.\n")
	assert.Contains(t, content, "\nAcceptance criteria:\n- idle 5 minutes survives\n")
	assert.Contains(t, content, "\nVerification plan:\n1. go test ./internal/auth/...\n")
	assert.True(t, strings.HasSuffix(content, "Next: task start root=main fix-login when you take it."), content)
}

func TestGetShowsAbsolutePathsFromOutsideTheCheckout(t *testing.T) {
	f := newFixture(t)
	res, err := tasktool.Tool(f.targets, tasks.AccessWrite).
		Run(tooldef.WithCwd(t.Context(), t.TempDir()), json.RawMessage(`{"action":"get","id":"docs"}`))
	require.NoError(t, err)
	assert.Contains(
		t,
		res.Content,
		"file: "+filepath.ToSlash(filepath.Join(resolved(t, f.root), "obsidian-tasks", "docs.md")),
	)
}

func TestGetOfAnEpicPointsAtItsChildren(t *testing.T) {
	f := newFixture(t)
	content, _ := f.run(t, `{"action":"get","id":"epic-auth"}`)
	assert.Contains(t, content, "Next: this is a container; task list parent=epic-auth for its children")
}

func TestLookupErrorsNameTheWayOut(t *testing.T) {
	f := newFixture(t)
	assert.EqualError(
		t,
		f.fail(t, `{"action":"get","id":"Nope"}`),
		`task: no task "nope"; call action=current or action=list to see the ids`,
	)
	assert.EqualError(
		t,
		f.fail(t, `{"action":"get"}`),
		"task: get needs an id; call action=current or action=list to see them",
	)
	assert.EqualError(
		t,
		f.fail(t, `{"action":"ship"}`),
		`task: unknown action "ship" (use current, list, get, create, update, start, done, block, reopen or note)`,
	)
	assert.EqualError(
		t,
		f.fail(t, `{"action":"start","root":"main"}`),
		"task: start needs an id; call action=current to see the open tasks",
	)
	assert.ErrorContains(t, f.fail(t, `{"action":"get","id":"docs","bogus":1}`), "invalid arguments")
}

func TestListFiltersAndCounts(t *testing.T) {
	f := newFixture(t)
	content, detail := f.run(t, `{"action":"list"}`)
	assert.Equal(t, "list (5)", detail)
	assert.True(
		t,
		strings.HasPrefix(
			content,
			"5 tasks (3 todo, 1 blocked, 1 done):\n- docs · todo · low · chore — Write the docs\n",
		),
		content,
	)

	content, detail = f.run(t, `{"action":"list","parent":"epic-auth"}`)
	assert.Equal(t, "list (1)", detail)
	assert.True(t, strings.HasPrefix(content, "1 of 5 tasks (parent=epic-auth):\n- fix-login"), content)

	content, _ = f.run(t, `{"action":"list","status":"done","tag":"auth"}`)
	assert.Contains(t, content, "No task matches status=done tag=auth; 5 in the registry (3 todo, 1 blocked, 1 done).")
}

func TestCreateWritesTheNoteAndSaysWhereItIs(t *testing.T) {
	f := newFixture(t)
	content, detail := f.run(
		t,
		`{"action":"create","root":"main","title":"Add SSE retry","type":"feature","priority":"medium","body":"Reconnect on EOF.","acceptance_criteria":["retries 3 times"]}`,
	)

	assert.Equal(t, "create add-sse-retry", detail)
	assert.Equal(
		t,
		"Created add-sse-retry (todo, medium) — Add SSE retry\nfile: obsidian-tasks/add-sse-retry.md (new, commit it)\n\nNext: task start root=main add-sse-retry when you take it.",
		content,
	)
	got, err := f.reg.Get("add-sse-retry")
	require.NoError(t, err)
	assert.Equal(t, "Reconnect on EOF.", got.Body)

	assert.EqualError(
		t,
		f.fail(t, `{"action":"create","root":"main","body":"x"}`),
		"task: create needs a title (and an id, unless the title reduces to one)",
	)
	assert.ErrorContains(
		t,
		f.fail(t, `{"action":"create","root":"main","title":"Dup","id":"docs"}`),
		"task: task already exists: docs",
	)
	assert.ErrorContains(
		t,
		f.fail(t, `{"action":"create","root":"main","title":"Bad","body":"## Notes"}`),
		"must not contain a `## ` heading",
	)
}

func TestStartNamesTheBranchAndWorktree(t *testing.T) {
	f := newFixture(t)
	content, detail := f.run(t, `{"action":"start","root":"main","id":"fix-login"}`)

	assert.Equal(t, "start fix-login", detail)
	assert.Contains(t, content, "Started fix-login (in_progress) — Fix login timeout\n")
	assert.Contains(
		t,
		content,
		"branch: bug/fix-login · worktree: .worktrees/fix-login (not created yet: git worktree add .worktrees/fix-login -b bug/fix-login, from "+resolved(
			t,
			f.root,
		)+")\n",
	)
	assert.Contains(t, content, "file: obsidian-tasks/fix-login.md (commit it with the work)\n")
	assert.Contains(
		t,
		content,
		"Next: do the work in .worktrees/fix-login on bug/fix-login; task note root=main fix-login to record progress; task done root=main fix-login (note: what landed) or task block root=main fix-login (note: what is in the way).",
	)

	require.NoError(t, os.MkdirAll(filepath.Join(f.root, ".worktrees", "fix-login"), 0o755))
	content, _ = f.run(t, `{"action":"start","root":"main","id":"fix-login"}`)
	assert.Contains(t, content, "fix-login was already in progress.\n")
	assert.Contains(t, content, "worktree: .worktrees/fix-login (exists)\n")

	assert.ErrorContains(
		t,
		f.fail(t, `{"action":"start","root":"main","id":"epic-auth"}`),
		"task: epic-auth is a container (epic or goal), not work; start one of its children — task list parent=epic-auth",
	)
	assert.EqualError(
		t,
		f.fail(t, `{"action":"start","root":"main","id":"old"}`),
		"task: old is done; task reopen old first if it must be redone",
	)
}

func TestDoneAndBlockNeedANoteAndRecordIt(t *testing.T) {
	f := newFixture(t)
	assert.EqualError(
		t,
		f.fail(t, `{"action":"done","root":"main","id":"docs"}`),
		"task: done needs a note: say what changed and where it landed (commit, merge, tag)",
	)
	assert.EqualError(
		t,
		f.fail(t, `{"action":"block","root":"main","id":"docs"}`),
		"task: block needs a note: say what is in the way and who or what clears it",
	)

	content, detail := f.run(t, `{"action":"done","root":"main","id":"docs","note":"merged as 1ab2c3d"}`)
	assert.Equal(t, "done docs", detail)
	assert.True(
		t,
		strings.HasPrefix(
			content,
			"Done docs (done) — Write the docs\nfile: obsidian-tasks/docs.md (commit it)\n\nNext: nothing, it is closed;",
		),
		content,
	)
	got, err := f.reg.Get("docs")
	require.NoError(t, err)
	assert.Contains(t, got.Body, "**Done (")
	assert.True(t, strings.HasSuffix(got.Body, ").** merged as 1ab2c3d"), got.Body)

	content, _ = f.run(t, `{"action":"block","root":"main","id":"fix-login","note":"needs the staging DB"}`)
	assert.Contains(t, content, "Blocked fix-login (blocked)")
	assert.Contains(t, content, "Next: task reopen root=main fix-login once the blocker clears")

	content, _ = f.run(t, `{"action":"reopen","root":"main","id":"fix-login"}`)
	assert.Contains(t, content, "Reopened fix-login (todo)")
	assert.Contains(t, content, "Next: task start root=main fix-login when you take it.")
}

func TestNoteAppendsWithoutMovingTheTask(t *testing.T) {
	f := newFixture(t)
	f.run(t, `{"action":"start","root":"main","id":"docs"}`)
	content, detail := f.run(t, `{"action":"note","root":"main","id":"docs","note":"outline is in"}`)
	assert.Equal(t, "note docs", detail)
	assert.True(
		t,
		strings.HasPrefix(content, "Noted on docs (in_progress).\nfile: obsidian-tasks/docs.md (commit it)\n"),
		content,
	)
	got, err := f.reg.Get("docs")
	require.NoError(t, err)
	assert.Equal(t, tasks.StatusInProgress, got.Status)
	assert.Contains(t, got.Body, ").** outline is in")
	assert.EqualError(t, f.fail(t, `{"action":"note","root":"main","id":"docs"}`), "task: note needs text in note")
}

func TestUpdateNamesWhatChanged(t *testing.T) {
	f := newFixture(t)
	content, detail := f.run(
		t,
		`{"action":"update","root":"main","id":"docs","title":"Write the user docs","tags":["docs"],"priority":"high"}`,
	)
	assert.Equal(t, "update docs", detail)
	assert.True(
		t,
		strings.HasPrefix(content, "Updated docs: title, priority, tags.\nfile: obsidian-tasks/docs.md (commit it)\n"),
		content,
	)
	got, err := f.reg.Get("docs")
	require.NoError(t, err)
	assert.Equal(t, "Write the user docs", got.Title)
	assert.Equal(t, "high", got.Priority)
	assert.Equal(t, []string{"docs"}, got.Tags)

	assert.ErrorContains(t, f.fail(t, `{"action":"update","root":"main","id":"docs"}`), "task: update changes nothing")
	assert.ErrorContains(
		t,
		f.fail(t, `{"action":"update","root":"main","id":"docs","priority":"urgent"}`),
		`task: invalid priority "urgent"`,
	)
}

func TestDetailNamesTheCallBeforeItRuns(t *testing.T) {
	tool := tasktool.Tool(nil, tasks.AccessWrite)
	for args, want := range map[string]string{
		`{}`: "current",
		`{"action":"list","status":"todo","tag":"x"}`:             "list status=todo tag=x",
		`{"action":"get","id":"Fix Login"}`:                       "get fix-login",
		`{"action":"create","root":"main","title":"Add retry"}`:   "create add-retry root=main",
		`{"action":"done","root":"main","id":"docs","note":"ok"}`: "done docs root=main",
		`{"action":"done","root":"main","id":"docs"}`:             "",
	} {
		assert.Equal(t, want, tool.DetailFromArgs(json.RawMessage(args)), args)
	}
}

// TestAccessLevelShapesTheTool pins what the model is offered at each
// permissions.tasks level: read lists reads only and says why, ask keeps
// every action and says each write is a question, write says nothing more.
// A write that reaches the read-level tool anyway is refused with the way
// out, not with a stack trace.
func TestAccessLevelShapesTheTool(t *testing.T) {
	f := newFixture(t)
	actions := func(tool tooldef.Tool) []string {
		return tool.Definition.Params.Properties["action"].(llm.Object)["enum"].([]string)
	}

	read := tasktool.Tool(f.targets, tasks.AccessRead)
	assert.Equal(t, []string{"current", "list", "get"}, actions(read))
	assert.Contains(t, read.Definition.Description, "permissions.tasks: read")
	assert.NotContains(t, read.Definition.Description, "- create:")
	res, err := read.Run(f.ctx(t), json.RawMessage(`{"action":"get","id":"fix-login"}`))
	require.NoError(t, err)
	assert.Contains(t, res.Content, "Fix login timeout")
	_, err = read.Run(f.ctx(t), json.RawMessage(`{"action":"start","root":"main","id":"docs"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permissions.tasks: read")
	assert.Contains(t, err.Error(), "describe the change")

	ask := tasktool.Tool(f.targets, tasks.AccessAsk)
	assert.Len(t, actions(ask), 10)
	assert.Contains(t, ask.Definition.Description, "asks the user before it lands")

	write := tasktool.Tool(f.targets, tasks.AccessWrite)
	assert.Len(t, actions(write), 10)
	assert.NotContains(t, write.Definition.Description, "asks the user")
	assert.Equal(t, write.Definition.Description, tasktool.Tool(f.targets, "").Definition.Description,
		"the empty level is the default, write")
}

// TestWritesNameTheirRoot pins the contract a write carries: the checkout
// whose ledger it changes, said by label. A write without one is refused with
// the labels that could have been said; a label nobody vouched for is
// refused the same way.
func TestWritesNameTheirRoot(t *testing.T) {
	f, _ := newTwoRootFixture(t)

	err := f.fail(t, `{"action":"note","id":"docs","note":"x"}`)
	assert.ErrorContains(t, err, "task: note writes the registry and needs root")
	assert.ErrorContains(t, err, "(known: main, other); this session started in main")

	err = f.fail(t, `{"action":"note","root":"elsewhere","id":"docs","note":"x"}`)
	assert.ErrorContains(t, err, `task: unknown registry root "elsewhere" (known: main, other)`)
}

// TestRootPicksTheRegistry pins what a label does: the write lands in that
// root's registry, the answer says where with an absolute path, reads stay
// on the launch checkout by default, and current names the known roots so
// the model never has to guess a label.
func TestRootPicksTheRegistry(t *testing.T) {
	f, other := newTwoRootFixture(t)

	content, _ := f.run(t, `{}`)
	assert.Contains(t, content, "Registries: main, other — writes name root; this session started in main.\n\n")

	content, _ = f.run(t, `{"action":"list","root":"other"}`)
	assert.Contains(t, content, "The registry is empty.")

	content, _ = f.run(t, `{"action":"create","root":"other","title":"Elsewhere"}`)
	assert.Contains(
		t,
		content,
		"file: "+filepath.ToSlash(
			filepath.Join(resolved(t, other), tasks.DefaultDir, "elsewhere.md"),
		)+" (new, commit it)",
	)
	assert.Contains(t, content, "Next: task start root=other elsewhere when you take it.")

	_, err := f.reg.Get("elsewhere")
	assert.Error(t, err, "the launch checkout's registry is untouched")

	content, _ = f.run(t, `{"action":"list"}`)
	assert.NotContains(t, content, "elsewhere", "reads default to the launch checkout")
}
