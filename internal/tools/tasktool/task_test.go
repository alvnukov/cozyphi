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

	"github.com/alvnukov/cozyphi/internal/tasks"
	"github.com/alvnukov/cozyphi/internal/tools/tasktool"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

type fixture struct {
	reg  *tasks.Registry
	root string
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
	return fixture{reg: reg, root: root}
}

func (f fixture) ctx(t *testing.T) context.Context {
	t.Helper()
	return tooldef.WithCwd(t.Context(), f.root)
}

func (f fixture) run(t *testing.T, args string) (string, string) {
	t.Helper()
	tool := tasktool.Tool(f.reg)
	require.Equal(t, "task", tool.Definition.Name)
	res, err := tool.Run(f.ctx(t), json.RawMessage(args))
	require.NoError(t, err)
	return res.Content, res.Detail
}

func (f fixture) fail(t *testing.T, args string) error {
	t.Helper()
	_, err := tasktool.Tool(f.reg).Run(f.ctx(t), json.RawMessage(args))
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
		strings.HasSuffix(content, "Next: task get <id> for the full note, then task start <id> to take it."),
		content,
	)
}

func TestCurrentOnAnEmptyRegistryPointsAtCreate(t *testing.T) {
	root := t.TempDir()
	f := fixture{reg: tasks.Open(root, filepath.Join(root, "obsidian-tasks")), root: root}
	content, _ := f.run(t, `{"action":"current"}`)
	assert.Contains(t, content, "No open tasks.")
	assert.Contains(t, content, "Next: task create")
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
	assert.True(t, strings.HasSuffix(content, "Next: task start fix-login when you take it."), content)
}

func TestGetShowsAbsolutePathsFromOutsideTheCheckout(t *testing.T) {
	f := newFixture(t)
	res, err := tasktool.Tool(f.reg).
		Run(tooldef.WithCwd(t.Context(), t.TempDir()), json.RawMessage(`{"action":"get","id":"docs"}`))
	require.NoError(t, err)
	assert.Contains(t, res.Content, "file: "+filepath.ToSlash(filepath.Join(f.root, "obsidian-tasks", "docs.md")))
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
		f.fail(t, `{"action":"start"}`),
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
		`{"action":"create","title":"Add SSE retry","type":"feature","priority":"medium","body":"Reconnect on EOF.","acceptance_criteria":["retries 3 times"]}`,
	)

	assert.Equal(t, "create add-sse-retry", detail)
	assert.Equal(
		t,
		"Created add-sse-retry (todo, medium) — Add SSE retry\nfile: obsidian-tasks/add-sse-retry.md (new, commit it)\n\nNext: task start add-sse-retry when you take it.",
		content,
	)
	got, err := f.reg.Get("add-sse-retry")
	require.NoError(t, err)
	assert.Equal(t, "Reconnect on EOF.", got.Body)

	assert.EqualError(
		t,
		f.fail(t, `{"action":"create","body":"x"}`),
		"task: create needs a title (and an id, unless the title reduces to one)",
	)
	assert.ErrorContains(
		t,
		f.fail(t, `{"action":"create","title":"Dup","id":"docs"}`),
		"task: task already exists: docs",
	)
	assert.ErrorContains(
		t,
		f.fail(t, `{"action":"create","title":"Bad","body":"## Notes"}`),
		"must not contain a `## ` heading",
	)
}

func TestStartNamesTheBranchAndWorktree(t *testing.T) {
	f := newFixture(t)
	content, detail := f.run(t, `{"action":"start","id":"fix-login"}`)

	assert.Equal(t, "start fix-login", detail)
	assert.Contains(t, content, "Started fix-login (in_progress) — Fix login timeout\n")
	assert.Contains(
		t,
		content,
		"branch: bug/fix-login · worktree: .worktrees/fix-login (not created yet: git worktree add .worktrees/fix-login -b bug/fix-login, from "+f.root+")\n",
	)
	assert.Contains(t, content, "file: obsidian-tasks/fix-login.md (commit it with the work)\n")
	assert.Contains(
		t,
		content,
		"Next: do the work in .worktrees/fix-login on bug/fix-login; task note fix-login to record progress; task done fix-login (note: what landed) or task block fix-login (note: what is in the way).",
	)

	require.NoError(t, os.MkdirAll(filepath.Join(f.root, ".worktrees", "fix-login"), 0o755))
	content, _ = f.run(t, `{"action":"start","id":"fix-login"}`)
	assert.Contains(t, content, "fix-login was already in progress.\n")
	assert.Contains(t, content, "worktree: .worktrees/fix-login (exists)\n")

	assert.ErrorContains(
		t,
		f.fail(t, `{"action":"start","id":"epic-auth"}`),
		"task: epic-auth is a container (epic or goal), not work; start one of its children — task list parent=epic-auth",
	)
	assert.EqualError(
		t,
		f.fail(t, `{"action":"start","id":"old"}`),
		"task: old is done; task reopen old first if it must be redone",
	)
}

func TestDoneAndBlockNeedANoteAndRecordIt(t *testing.T) {
	f := newFixture(t)
	assert.EqualError(
		t,
		f.fail(t, `{"action":"done","id":"docs"}`),
		"task: done needs a note: say what changed and where it landed (commit, merge, tag)",
	)
	assert.EqualError(
		t,
		f.fail(t, `{"action":"block","id":"docs"}`),
		"task: block needs a note: say what is in the way and who or what clears it",
	)

	content, detail := f.run(t, `{"action":"done","id":"docs","note":"merged as 1ab2c3d"}`)
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

	content, _ = f.run(t, `{"action":"block","id":"fix-login","note":"needs the staging DB"}`)
	assert.Contains(t, content, "Blocked fix-login (blocked)")
	assert.Contains(t, content, "Next: task reopen fix-login once the blocker clears")

	content, _ = f.run(t, `{"action":"reopen","id":"fix-login"}`)
	assert.Contains(t, content, "Reopened fix-login (todo)")
	assert.Contains(t, content, "Next: task start fix-login when you take it.")
}

func TestNoteAppendsWithoutMovingTheTask(t *testing.T) {
	f := newFixture(t)
	f.run(t, `{"action":"start","id":"docs"}`)
	content, detail := f.run(t, `{"action":"note","id":"docs","note":"outline is in"}`)
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
	assert.EqualError(t, f.fail(t, `{"action":"note","id":"docs"}`), "task: note needs text in note")
}

func TestUpdateNamesWhatChanged(t *testing.T) {
	f := newFixture(t)
	content, detail := f.run(
		t,
		`{"action":"update","id":"docs","title":"Write the user docs","tags":["docs"],"priority":"high"}`,
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

	assert.ErrorContains(t, f.fail(t, `{"action":"update","id":"docs"}`), "task: update changes nothing")
	assert.ErrorContains(
		t,
		f.fail(t, `{"action":"update","id":"docs","priority":"urgent"}`),
		`task: invalid priority "urgent"`,
	)
}

func TestDetailNamesTheCallBeforeItRuns(t *testing.T) {
	tool := tasktool.Tool(nil)
	for args, want := range map[string]string{
		`{}`: "current",
		`{"action":"list","status":"todo","tag":"x"}`: "list status=todo tag=x",
		`{"action":"get","id":"Fix Login"}`:           "get fix-login",
		`{"action":"create","title":"Add retry"}`:     "create add-retry",
		`{"action":"done","id":"docs","note":"ok"}`:   "done docs",
		`{"action":"done","id":"docs"}`:               "",
	} {
		assert.Equal(t, want, tool.DetailFromArgs(json.RawMessage(args)), args)
	}
}
