package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/usage"
)

// storageField asks one session what it can say about what it keeps, through
// the same path the harness tool takes.
func storageField(t *testing.T, c *Controller, key string) diag.Field {
	t.Helper()
	explained, err := c.diagnostics.Explain(t.Context(), diag.CategoryStorage, key)
	require.NoError(t, err)
	return explained.Field
}

// storageLayers reduces one storage snapshot to what each field says, with
// the observation time left out: the registry stamps a real clock, so two
// answers of one unchanged state are equal in everything but that.
func storageLayers(t *testing.T, c *Controller) map[string][3]diag.Observation {
	t.Helper()
	snapshot, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryStorage)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	layers := make(map[string][3]diag.Observation, len(snapshot.Categories[0].Fields))
	for _, field := range snapshot.Categories[0].Fields {
		layers[field.Key] = [3]diag.Observation{field.Configured, field.Loaded, field.Effective}
	}
	return layers
}

// storageSession stands up a developer session with every store wired: a
// corpus holding one memory, a shared history with one use recorded in it,
// and a transcript this workspace's layout fixes the directory of. The
// registry is the one store a fresh temporary directory has none of, which
// is itself worth observing.
func storageSession(t *testing.T, memoryFile string) (*Controller, *project.Project, *usage.Store) {
	t.Helper()
	proj, cwd := developerProject(t)
	// The runtime resolves symlinks out of a workspace path before it keys
	// anything by it, and a temporary directory on macOS is reached through
	// one. Discovering the same workspace under the spelling the runtime will
	// use is what makes the corpus seeded below the corpus it opens.
	cwd, err := filepath.EvalSymlinks(cwd)
	require.NoError(t, err)
	if cwd != proj.Root() {
		proj, err = project.Discover(cwd)
		require.NoError(t, err)
	}
	if memoryFile != "" {
		require.NoError(t, os.MkdirAll(proj.MemoryDir(), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(proj.MemoryDir(), "release-freeze.md"),
			[]byte(memoryFile), 0o600))
	}
	history, err := usage.Open(proj.Global().UsageFile())
	require.NoError(t, err)

	rt, err := NewRuntime(proj, history)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })
	require.NoError(t, rt.GrantDeveloperMode())
	ws, err := rt.Workspace(cwd)
	require.NoError(t, err)
	c, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	return c, proj, history
}

// Every locator is an anchor the layout fixes plus what sits below it, and
// the segments that merely encode another path are filled in rather than
// spelled out. This is the assertion the whole category turns on: the home
// collapse cannot reach inside a directory named after a working directory,
// because the separators are already gone.
func TestASessionReportsWhereItsStoresAreWithoutSpellingOutAnEncodedPath(t *testing.T) {
	c, proj, _ := storageSession(t, "")
	encoded := filepath.Base(proj.SessionDir())
	require.Contains(t, encoded, "-", "the layout does encode the working directory into that name")

	transcript := storageField(t, c, diag.KeySessionsLocation)
	assert.Equal(t, "~/.cozyphi/session", transcript.Configured.Value.Str)
	assert.Equal(t, "~/.cozyphi/session/<workspace>/<session>.jsonl", transcript.Loaded.Value.Str)

	corpus := storageField(t, c, diag.KeyMemoryLocation)
	assert.Equal(t, "~/.claude/projects", corpus.Configured.Value.Str)
	assert.Equal(t, "~/.claude/projects/<corpus>/memory", corpus.Loaded.Value.Str)

	history := storageField(t, c, diag.KeyUsageLocation)
	assert.Equal(t, "~/.cozyphi/usage.json", history.Loaded.Value.Str,
		"nothing in this one encodes a path, so it is reported as it is")

	snapshot, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryStorage)
	require.NoError(t, err)
	rendered := fmt.Sprintf("%+v", snapshot)
	assert.NotContains(t, rendered, encoded)
	assert.NotContains(t, rendered, c.SessionID())
	assert.NotContains(t, rendered, c.SessionFile())
}

// A session opened for persistence holds its file from the start and writes
// on its first assistant turn. Both states are real and a reader needs to
// tell them apart, so the three layers carry three different answers.
func TestASessionObservesItsTranscriptBeforeAndAfterItIsWritten(t *testing.T) {
	c, _, _ := storageSession(t, "")

	pending := storageField(t, c, diag.KeySessionsState)
	assert.Equal(t, string(diag.SessionsPersistent), pending.Configured.Value.Str)
	assert.Equal(t, string(diag.SessionsFile), pending.Loaded.Value.Str)
	assert.Equal(t, string(diag.SessionsPending), pending.Effective.Value.Str)
	assert.Equal(t, diag.StateUnset, storageField(t, c, diag.KeySessionsEntries).Effective.State,
		"how much would survive this process is not yet a number")

	require.NoError(t, c.engine.Session().Append(llm.Message{
		Role: llm.RoleAssistant, Content: "observed in a developer process",
	}))

	written := storageField(t, c, diag.KeySessionsState)
	assert.Equal(t, string(diag.SessionsPersisted), written.Effective.Value.Str)
	entries := storageField(t, c, diag.KeySessionsEntries)
	assert.Equal(t, diag.StatePresent, entries.Effective.State)
	assert.Positive(t, entries.Effective.Value.Int)
	assert.NotEqual(t, pending.Revision, written.Revision)
}

// A workspace outside Git keys its own corpus, and a fresh one has nothing
// in it. "Empty" is an answer, and it is not the answer a broken corpus or
// an unbuilt index would give.
func TestAWorkspaceOutsideGitIsToldItsMemoriesAreItsOwn(t *testing.T) {
	c, _, _ := storageSession(t, "")

	corpus := storageField(t, c, diag.KeyMemoryCorpus)
	assert.Equal(t, string(diag.StorageIdentityWorkspace), corpus.Configured.Value.Str)
	assert.Equal(t, string(diag.MemoryCorpusOwn), corpus.Loaded.Value.Str)
	assert.False(t, corpus.Effective.Value.Bool, "nothing else is reading these memories")

	state := storageField(t, c, diag.KeyMemoryState)
	assert.Equal(t, string(diag.MemoryOpen), state.Loaded.Value.Str)
	assert.Equal(t, string(diag.MemoryEmpty), state.Effective.Value.Str)
	assert.Equal(t, int64(0), storageField(t, c, diag.KeyMemoryCount).Loaded.Value.Int)
	assert.True(t, storageField(t, c, diag.KeyMemoryIndex).Loaded.Value.Bool)
}

// A memory holds what someone told the agent once, and a usage key is built
// from a directory and a name someone typed. Neither has a way out through
// this category — what leaves is counts.
func TestNoMemoryOrHistoryKeyReachesTheAnswer(t *testing.T) {
	const sentinel = "sk-live-STORAGE-SENTINEL"
	c, proj, history := storageSession(t, "---\nname: release-freeze\n"+
		"description: the key is "+sentinel+"\nmetadata:\n  type: project\n---\n\n"+
		"The key is "+sentinel+".\n")
	usage.Memory{Store: history, Dir: proj.MemoryDir()}.Use("release-freeze")
	require.NoError(t, history.Record(usage.Models, sentinel))

	require.Equal(t, int64(1), storageField(t, c, diag.KeyMemoryCount).Loaded.Value.Int,
		"the corpus really does hold it, which is why the rest is worth checking")
	assert.Equal(t, []string{"project=1"}, storageField(t, c, diag.KeyMemoryKinds).Loaded.Value.List)
	assert.Equal(t, []string{"models=1", "memories=1"},
		storageField(t, c, diag.KeyUsageScopes).Loaded.Value.List)

	snapshot, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryStorage)
	require.NoError(t, err)
	rendered := fmt.Sprintf("%+v", snapshot)
	assert.NotContains(t, rendered, sentinel)
	assert.NotContains(t, rendered, "release-freeze")
	assert.NotContains(t, rendered, proj.MemoryDir())
}

// A repository with no registry is an answer, and the count of tasks in one
// is a question this view refuses: the registry keeps none in memory, and
// producing one would be a read of the directory.
func TestAWorkspaceWithoutATaskRegistrySaysSoAndCountsNothing(t *testing.T) {
	c, _, _ := storageSession(t, "")

	state := storageField(t, c, diag.KeyTasksState)
	assert.Equal(t, string(diag.TasksAbsent), state.Loaded.Value.Str)
	assert.False(t, state.Effective.Value.Bool)

	location := storageField(t, c, diag.KeyTasksLocation)
	assert.Equal(t, "<repo>/obsidian-tasks", location.Configured.Value.Str,
		"where one would live is still an answer")
	assert.Equal(t, diag.StateUnset, location.Loaded.State)
	assert.Equal(t, string(diag.StorageIdentityRepository), location.Effective.Value.Str)

	count := storageField(t, c, diag.KeyTasksCount)
	assert.Equal(t, diag.StateNotApplicable, count.Loaded.State)
	assert.NotEmpty(t, count.Loaded.Source.Ref)
}

// Every session of one workspace shares the corpus, the registry and the
// history, and each writes its own transcript. A session must answer for its
// own transcript rather than for whichever session is active.
func TestEachSessionAnswersForItsOwnTranscriptAndTheSharedRest(t *testing.T) {
	first, _, _ := storageSession(t, "")
	rt := first.runtime
	ws, err := rt.Workspace(first.cwd)
	require.NoError(t, err)
	second, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	require.NotEqual(t, first.SessionID(), second.SessionID())

	require.NoError(t, first.engine.Session().Append(llm.Message{
		Role: llm.RoleAssistant, Content: "written in the first session only",
	}))

	assert.Equal(t, string(diag.SessionsPersisted),
		storageField(t, first, diag.KeySessionsState).Effective.Value.Str)
	assert.Equal(t, string(diag.SessionsPending),
		storageField(t, second, diag.KeySessionsState).Effective.Value.Str,
		"one transcript belongs to one conversation")

	for _, key := range []string{diag.KeyMemoryLocation, diag.KeyTasksLocation, diag.KeyUsageLocation} {
		assert.Equal(t, storageField(t, first, key).Loaded, storageField(t, second, key).Loaded, key)
	}
}

// Reading is reading: no transcript is flushed, no memory index rebuilt, no
// note parsed and no use recorded. Asking what a session keeps must not
// change what it keeps or what the next turn recalls.
func TestObservingStorageLeavesEveryStoreExactlyAsItWas(t *testing.T) {
	c, proj, history := storageSession(t, "---\nname: release-freeze\n"+
		"description: the freeze is on\nmetadata:\n  type: project\n---\n\nHold the line.\n")
	usage.Memory{Store: history, Dir: proj.MemoryDir()}.Use("release-freeze")

	transcript, err := os.ReadFile(c.SessionFile())
	require.ErrorIs(t, err, os.ErrNotExist, "nothing has been flushed yet, and nothing here may flush it")
	corpus, err := os.ReadDir(proj.MemoryDir())
	require.NoError(t, err)
	usageFile, err := os.ReadFile(proj.Global().UsageFile())
	require.NoError(t, err)
	seen, at := history.Seen(usage.Memories, proj.MemoryDir()+"\x00release-freeze")

	first := storageLayers(t, c)
	for range 3 {
		assert.Equal(t, first, storageLayers(t, c),
			"the same answer every time, from what each store already holds")
	}

	_, err = os.ReadFile(c.SessionFile())
	assert.ErrorIs(t, err, os.ErrNotExist)
	after, err := os.ReadDir(proj.MemoryDir())
	require.NoError(t, err)
	assert.Len(t, after, len(corpus))
	afterUsage, err := os.ReadFile(proj.Global().UsageFile())
	require.NoError(t, err)
	assert.Equal(t, usageFile, afterUsage, "the history is byte for byte what it was")
	count, when := history.Seen(usage.Memories, proj.MemoryDir()+"\x00release-freeze")
	assert.Equal(t, seen, count)
	assert.Equal(t, at, when)
	assert.Empty(t, transcript)
}
