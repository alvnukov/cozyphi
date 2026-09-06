package tasks_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tasks"
)

// The registry is tracked in the repository, so where it sits inside one is
// the whole of what a reader needs — and the absolute path on this machine,
// which the registry does hold, is not part of it.
func TestTheRegistryIsReportedRelativeToItsOwnRepository(t *testing.T) {
	root := t.TempDir()
	cfg := "task_registry:\n    backend: obsidian\n    obsidian:\n        path: notes/tasks\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, tasks.ConfigFile), []byte(cfg), 0o600))
	registry, err := tasks.Discover(root)
	require.NoError(t, err)
	require.NotNil(t, registry)

	facts := tasks.Observe(registry, tasks.ObserveDiscover(err))

	assert.True(t, facts.Known)
	assert.True(t, facts.Attempted)
	assert.True(t, facts.Found)
	assert.False(t, facts.Failed)
	assert.Equal(t, filepath.Join("notes", "tasks"), facts.Dir)
	assert.Equal(t, tasks.DefaultDir, facts.DefaultDir, "so a default is legible as one")
	assert.NotContains(t, fmt.Sprintf("%+v", facts), root,
		"the registry holds the absolute path; the observation does not")
}

// Discover hands back nil both for a repository with no registry and for a
// config it refused, so the registry alone cannot tell them apart. The three
// answers travel beside it — and none of them is "there are no tasks".
func TestARefusedConfigIsNotARepositoryWithoutARegistry(t *testing.T) {
	escaping := t.TempDir()
	cfg := "task_registry:\n    backend: obsidian\n    obsidian:\n        path: ../elsewhere\n"
	require.NoError(t, os.WriteFile(filepath.Join(escaping, tasks.ConfigFile), []byte(cfg), 0o600))
	refused, err := tasks.Discover(escaping)
	require.Error(t, err)
	require.Nil(t, refused)

	empty, emptyErr := tasks.Discover(t.TempDir())
	require.NoError(t, emptyErr)
	require.Nil(t, empty)

	for _, tc := range []struct {
		name      string
		registry  *tasks.Registry
		load      tasks.DiscoverFacts
		known     bool
		attempted bool
		failed    bool
	}{
		{name: "nobody looked"},
		{
			name: "the repository has none", registry: empty, load: tasks.ObserveDiscover(emptyErr),
			known: true, attempted: true,
		},
		{
			name: "the config was refused", registry: refused, load: tasks.ObserveDiscover(err),
			known: true, attempted: true, failed: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := tasks.Observe(tc.registry, tc.load)
			assert.Equal(t, tc.known, facts.Known)
			assert.Equal(t, tc.attempted, facts.Attempted)
			assert.Equal(t, tc.failed, facts.Failed)
			assert.False(t, facts.Found)
			assert.Empty(t, facts.Dir, "no registry is in force, so it is nowhere")
			assert.Equal(t, tasks.DefaultDir, facts.DefaultDir,
				"where one would live is a property of the build, answerable with no registry at all")
		})
	}
}

// The error a refused config returns quotes the path the config named. Only
// the fact of failure is kept.
func TestTheDiscoveryErrorIsDroppedRatherThanCarried(t *testing.T) {
	root := t.TempDir()
	const secret = "../../sk-live-TASKS-SENTINEL"
	cfg := "task_registry:\n    backend: obsidian\n    obsidian:\n        path: " + secret + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, tasks.ConfigFile), []byte(cfg), 0o600))
	_, err := tasks.Discover(root)
	require.ErrorContains(t, err, secret,
		"the error still has it, which is why it is worth checking that the observation does not")

	facts := tasks.ObserveDiscover(err)
	assert.True(t, facts.Failed)
	assert.NotContains(t, fmt.Sprintf("%+v", facts), secret)
}

// Reading is reading: the directory is not listed, no note is opened or
// parsed, and nothing is created — which is also why no count of tasks is
// reported at all.
func TestObservingTheRegistryReadsNoNoteAndCreatesNothing(t *testing.T) {
	registry, root := newRegistry(t)
	create(t, registry, tasks.Draft{ID: "one", Title: "первая", Status: tasks.StatusTodo})
	dir := filepath.Join(root, tasks.DefaultDir)
	before, err := os.ReadDir(dir)
	require.NoError(t, err)

	first := tasks.Observe(registry, tasks.ObserveDiscover(nil))
	for range 3 {
		assert.Equal(t, first, tasks.Observe(registry, tasks.ObserveDiscover(nil)))
	}

	after, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, after, len(before), "the registry directory is untouched")

	notes, _, err := registry.List()
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.NotContains(t, fmt.Sprintf("%+v", first), "первая",
		"a title is what someone typed, and none of it leaves here")
	assert.NotContains(t, fmt.Sprintf("%+v", first), "one")
}
