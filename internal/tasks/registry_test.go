package tasks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tasks"
)

func newRegistry(t *testing.T) (*tasks.Registry, string) {
	t.Helper()
	root := t.TempDir()
	return tasks.Open(root, filepath.Join(root, tasks.DefaultDir)), root
}

func create(t *testing.T, r *tasks.Registry, d tasks.Draft) tasks.Task {
	t.Helper()
	task, err := r.Create(d)
	require.NoError(t, err)
	return task
}

func TestDiscoverFollowsTheHelperConfig(t *testing.T) {
	t.Run("config names the directory", func(t *testing.T) {
		root := t.TempDir()
		cfg := "task_registry:\n    backend: obsidian\n    obsidian:\n        path: notes/tasks\n"
		require.NoError(t, os.WriteFile(filepath.Join(root, tasks.ConfigFile), []byte(cfg), 0o600))
		r, err := tasks.Discover(root)
		require.NoError(t, err)
		require.NotNil(t, r)
		assert.Equal(t, filepath.Join(root, "notes", "tasks"), r.Dir())
		assert.Equal(t, root, r.Root())
	})
	t.Run("config without a path means the default", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(
			t,
			os.WriteFile(
				filepath.Join(root, tasks.ConfigFile),
				[]byte("task_registry:\n    backend: obsidian\n"),
				0o600,
			),
		)
		r, err := tasks.Discover(root)
		require.NoError(t, err)
		require.NotNil(t, r)
		assert.Equal(t, filepath.Join(root, tasks.DefaultDir), r.Dir())
	})
	t.Run("default directory without a config", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, tasks.DefaultDir), 0o755))
		r, err := tasks.Discover(root)
		require.NoError(t, err)
		require.NotNil(t, r)
	})
	t.Run("nothing means no registry", func(t *testing.T) {
		r, err := tasks.Discover(t.TempDir())
		require.NoError(t, err)
		assert.Nil(t, r)
	})
	t.Run("another backend is not ours", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(
			t,
			os.WriteFile(filepath.Join(root, tasks.ConfigFile), []byte("task_registry:\n    backend: lean\n"), 0o600),
		)
		require.NoError(t, os.Mkdir(filepath.Join(root, tasks.DefaultDir), 0o755))
		r, err := tasks.Discover(root)
		require.NoError(t, err)
		assert.Nil(t, r)
	})
	t.Run("a path outside the repository is refused", func(t *testing.T) {
		root := t.TempDir()
		cfg := "task_registry:\n    obsidian:\n        path: ../elsewhere\n"
		require.NoError(t, os.WriteFile(filepath.Join(root, tasks.ConfigFile), []byte(cfg), 0o600))
		_, err := tasks.Discover(root)
		require.ErrorContains(t, err, "escapes the repository")
	})
}

func TestCreateWritesAHelperReadableNote(t *testing.T) {
	r, root := newRegistry(t)
	task := create(t, r, tasks.Draft{
		ID:                 "Fix Login: Timeout",
		Title:              "Fix the login timeout",
		Body:               "Sessions drop after 30s.",
		Type:               "Bug",
		Priority:           "High",
		Tags:               []string{"Auth"},
		AcceptanceCriteria: []string{"login survives 5 minutes idle", " "},
		VerificationPlan:   []string{"go test ./internal/auth/..."},
	})

	assert.Equal(t, "fix-login-timeout", task.ID, "the id is normalized the way the helper normalizes it")
	assert.Equal(t, filepath.Join(root, tasks.DefaultDir, "fix-login-timeout.md"), task.Path)
	assert.Equal(t, tasks.StatusTodo, task.Status)
	assert.Equal(t, "bug", task.Type)
	assert.Equal(t, "high", task.Priority)
	assert.Equal(t, []string{"auth"}, task.Tags)
	assert.Equal(t, []string{"login survives 5 minutes idle"}, task.AcceptanceCriteria)
	assert.False(t, task.CreatedAt.IsZero())

	data, err := os.ReadFile(task.Path)
	require.NoError(t, err)
	text := string(data)
	assert.True(
		t,
		strings.HasPrefix(
			text,
			"---\nid: fix-login-timeout\ntitle: Fix the login timeout\nstatus: todo\npriority: high\ntask_type: bug\ntags:\n    - auth\n",
		),
		text,
	)
	assert.Contains(t, text, "\n## Body\n\nSessions drop after 30s.\n")
	assert.Contains(t, text, "\n## Acceptance Criteria\n\n- login survives 5 minutes idle\n")
	assert.Contains(t, text, "\n## Verification Plan\n\n1. go test ./internal/auth/...\n")

	got, err := r.Get("fix-login-timeout")
	require.NoError(t, err)
	got.Path = task.Path
	assert.Equal(t, task, got)
}

func TestCreateDerivesTheIDFromTheTitle(t *testing.T) {
	r, _ := newRegistry(t)
	task := create(t, r, tasks.Draft{Title: "Add retry to the SSE client"})
	assert.Equal(t, "add-retry-to-the-sse-client", task.ID)
	assert.Equal(t, "task", task.Type, "a task without a type is a plain task, so a branch can still be named")

	_, err := r.Create(tasks.Draft{Title: "Только кириллица"})
	require.ErrorContains(t, err, "an id is required")
}

func TestCreateRefusals(t *testing.T) {
	r, _ := newRegistry(t)
	create(t, r, tasks.Draft{ID: "one", Title: "One"})

	_, err := r.Create(tasks.Draft{ID: "one", Title: "Again"})
	require.ErrorIs(t, err, tasks.ErrExists)

	_, err = r.Create(tasks.Draft{ID: "two", Title: "Two", ParentID: "missing"})
	require.ErrorContains(t, err, `parent "missing" is not in the registry`)

	_, err = r.Create(tasks.Draft{ID: "two", Title: "Two", Body: "text\n## Notes\nlost"})
	require.ErrorIs(t, err, tasks.ErrHeading)

	_, err = r.Create(tasks.Draft{ID: "two", Title: "Two", Priority: "urgent"})
	require.ErrorContains(t, err, `invalid priority "urgent" (use critical, high, medium, low)`)

	_, err = r.Create(tasks.Draft{ID: "two", Title: "Two", Type: "Feature Work"})
	require.ErrorContains(t, err, "invalid type")

	_, err = r.Create(tasks.Draft{ID: "two"})
	require.ErrorContains(t, err, "title is required")
}

func TestGetReportsAMissingTask(t *testing.T) {
	r, _ := newRegistry(t)
	_, err := r.Get("nope")
	require.ErrorIs(t, err, tasks.ErrNotFound)
	list, diags, err := r.List()
	require.NoError(t, err)
	assert.Empty(t, list)
	assert.Empty(t, diags, "a registry directory that does not exist yet is empty, not broken")
}

func TestCurrentRanksLikeTheHelper(t *testing.T) {
	r, _ := newRegistry(t)
	create(t, r, tasks.Draft{ID: "low-todo", Title: "Low todo", Priority: "low"})
	create(t, r, tasks.Draft{ID: "crit-todo", Title: "Critical todo", Priority: "critical"})
	create(t, r, tasks.Draft{ID: "epic", Title: "Epic", Type: "epic", Priority: "critical"})
	create(t, r, tasks.Draft{ID: "goal", Title: "Goal", Tags: []string{"goal"}, Priority: "critical"})
	create(t, r, tasks.Draft{ID: "finished", Title: "Finished", Status: tasks.StatusDone})
	create(t, r, tasks.Draft{ID: "stuck", Title: "Stuck", Status: tasks.StatusBlocked})
	create(t, r, tasks.Draft{ID: "going", Title: "Going", Status: tasks.StatusInProgress, Priority: "low"})
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir(), "broken.md"), []byte("no frontmatter"), 0o600))

	cur, err := r.Current()
	require.NoError(t, err)

	assert.Equal(t, []string{"going", "crit-todo", "low-todo"}, ids(cur.Ready),
		"in_progress first, then by priority; epics, goals, done and blocked are not work")
	assert.Equal(t, []string{"stuck"}, ids(cur.Blocked))
	require.Len(t, cur.Diagnostics, 1)
	assert.Equal(t, "broken.md", cur.Diagnostics[0].File)
	assert.ErrorContains(t, cur.Diagnostics[0].Err, "opening ---")
}

func TestSetStatusRecordsHistoryUnderBoldLabels(t *testing.T) {
	r, _ := newRegistry(t)
	create(t, r, tasks.Draft{ID: "job", Title: "Job", Type: "feature", Body: "Do the thing."})

	started, err := r.SetStatus("job", tasks.StatusInProgress, "")
	require.NoError(t, err)
	assert.Equal(t, tasks.StatusInProgress, started.Status)
	assert.Equal(t, "feature/job", started.Branch)
	assert.Equal(t, ".worktrees/job", started.WorktreePath)
	assert.Equal(t, "Do the thing.", started.Body, "no text, no paragraph")

	today := time.Now().Format("2006-01-02")
	noted, err := r.Note("job", "half way; the parser is in")
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(noted.Body, "\n\n**Note ("+today+").** half way; the parser is in"), noted.Body)

	done, err := r.SetStatus("job", tasks.StatusDone, "merged as 1ab2c3d")
	require.NoError(t, err)
	assert.Equal(t, tasks.StatusDone, done.Status)
	assert.True(t, strings.HasSuffix(done.Body, "\n\n**Done ("+today+").** merged as 1ab2c3d"), done.Body)
	assert.True(t, done.UpdatedAt.After(done.CreatedAt) || done.UpdatedAt.Equal(done.CreatedAt))

	reread, err := r.Get("job")
	require.NoError(t, err)
	assert.Equal(t, done.Body, reread.Body)
	assert.Equal(t, "feature/job", reread.Branch, "the branch survives later transitions")

	blocked, err := r.SetStatus("job", tasks.StatusBlocked, "waiting on\n## review")
	require.ErrorIs(t, err, tasks.ErrHeading)
	assert.Empty(t, blocked.ID)

	reopened, err := r.SetStatus("job", tasks.StatusTodo, "needs another pass")
	require.NoError(t, err)
	assert.Contains(t, reopened.Body, "**Reopened ("+today+").** needs another pass")

	_, err = r.SetStatus("job", "soon", "")
	require.ErrorContains(t, err, `invalid status "soon"`)
	_, err = r.Note("job", "  ")
	require.ErrorContains(t, err, "needs text")
}

func TestUpdateChangesOnlyWhatThePatchNames(t *testing.T) {
	r, _ := newRegistry(t)
	create(t, r, tasks.Draft{ID: "parent", Title: "Parent", Type: "epic"})
	before := create(t, r, tasks.Draft{
		ID: "child", Title: "Child", Body: "body", Priority: "low", Tags: []string{"a"},
		AcceptanceCriteria: []string{"one"},
	})

	title := "Child, renamed"
	parent := "parent"
	after, err := r.Update(
		"child",
		tasks.Patch{Title: &title, ParentID: &parent, Tags: []string{}, VerificationPlan: []string{"go test ./..."}},
	)
	require.NoError(t, err)
	assert.Equal(t, "Child, renamed", after.Title)
	assert.Equal(t, "parent", after.ParentID)
	assert.Empty(t, after.Tags, "an empty list clears")
	assert.Equal(t, []string{"one"}, after.AcceptanceCriteria, "a nil list keeps")
	assert.Equal(t, []string{"go test ./..."}, after.VerificationPlan)
	assert.Equal(t, "body", after.Body)
	assert.Equal(t, "low", after.Priority)
	assert.Equal(t, before.CreatedAt, after.CreatedAt)

	self := "child"
	_, err = r.Update("child", tasks.Patch{ParentID: &self})
	require.ErrorContains(t, err, "its own parent")
	_, err = r.Update("ghost", tasks.Patch{Title: &title})
	require.ErrorIs(t, err, tasks.ErrNotFound)
}

func TestHelpers(t *testing.T) {
	assert.Equal(t, "ct-001", tasks.NormalizeID(" CT-001 "))
	assert.Equal(t, "a-b.c_d", tasks.NormalizeID("--A  b.c_d!!"))
	assert.Empty(t, tasks.NormalizeID("..."))
	assert.Equal(t, ".worktrees/x", tasks.WorktreeFor("X"))
	assert.Equal(t, "task/x", tasks.BranchFor(tasks.Task{ID: "x"}))
	assert.Equal(t, "bug/x", tasks.BranchFor(tasks.Task{ID: "x", Type: "bug"}))
	assert.True(t, tasks.Task{Type: "epic", Status: tasks.StatusTodo}.IsEpic())
	assert.False(t, tasks.Task{Type: "epic", Status: tasks.StatusTodo}.Executable())
	assert.True(t, tasks.Task{Status: tasks.StatusInProgress}.Executable())
}

func ids(list []tasks.Task) []string {
	out := make([]string, len(list))
	for i, task := range list {
		out[i] = task.ID
	}
	return out
}
