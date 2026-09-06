package usage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// One of these scopes keys its items by the directory a memory corpus
// belongs to, and another by a model name someone typed. Counts are the only
// thing that may leave, and they are reported against the vocabulary this
// build records so an empty scope is legible as empty.
func TestNoHistoryKeyReachesTheObservation(t *testing.T) {
	const secret = "/Users/someone/src/sk-live-USAGE-SENTINEL"
	path := filepath.Join(t.TempDir(), "usage.json")
	store, err := Open(path)
	require.NoError(t, err)
	memories := Memory{Store: store, Dir: secret}
	memories.Use("release-freeze")
	memories.Use("who-the-user-is")
	require.NoError(t, store.Record(Models, "internal-preview-model"))

	facts := Observe(store)

	assert.True(t, facts.Known)
	assert.True(t, facts.Persistent)
	assert.False(t, facts.OpenFailed)
	assert.Equal(t, 3, facts.Items)
	assert.Equal(t, Scopes(), facts.Vocabulary)
	require.Len(t, facts.Scopes, len(Scopes()))
	for _, scope := range facts.Scopes {
		switch scope.Scope {
		case Memories:
			assert.Equal(t, 2, scope.Items)
		case Models:
			assert.Equal(t, 1, scope.Items)
		default:
			assert.Zero(t, scope.Items, scope.Scope)
		}
	}

	rendered := fmt.Sprintf("%+v", facts)
	assert.NotContains(t, rendered, secret)
	assert.NotContains(t, rendered, "release-freeze")
	assert.NotContains(t, rendered, "internal-preview-model")
	assert.NotContains(t, rendered, path, "not even the file it is kept in")
}

// A history that failed to parse leaves a usable, empty store: ranking
// degrades rather than failing. From every picker in the process that looks
// exactly like a fresh install, and this is the only place the difference is
// recorded — which is why the store keeps it rather than the caller.
func TestAHistoryThatFailedToLoadIsNotAFreshInstall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o600))

	broken, err := Open(path)
	require.Error(t, err)
	require.NotNil(t, broken, "the store is usable either way")
	failed := Observe(broken)
	assert.True(t, failed.OpenFailed)
	assert.True(t, failed.Persistent)
	assert.Zero(t, failed.Items)

	fresh, err := Open(filepath.Join(t.TempDir(), "usage.json"))
	require.NoError(t, err)
	observed := Observe(fresh)
	assert.False(t, observed.OpenFailed, "a file that is not there yet is not a file that failed")
	assert.Zero(t, observed.Items)
	assert.NotEqual(t, failed.Revision, observed.Revision)
}

// A store with no path ranks this process's own choices and forgets them.
// It is stored nowhere, which is an answer rather than a missing one.
func TestAnInMemoryHistoryIsReportedAsBackedByNothing(t *testing.T) {
	store, err := Open("")
	require.NoError(t, err)
	require.NoError(t, store.Record(Palette, "toggle-theme"))

	facts := Observe(store)
	assert.True(t, facts.Known)
	assert.False(t, facts.Persistent)
	assert.Equal(t, 1, facts.Items)
}

// Reading is reading: the file is neither re-read nor written, no use is
// recorded and nothing is pruned — so asking how the pickers are ranked
// cannot change how the next one orders itself.
func TestObservingTheHistoryRecordsNothingAndWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	store, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, store.Record(Skills, "release"))
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	seen, at := store.Seen(Skills, "release")

	first := Observe(store)
	for range 3 {
		assert.Equal(t, first, Observe(store))
	}

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after, "the history is byte for byte what it was")
	count, when := store.Seen(Skills, "release")
	assert.Equal(t, seen, count, "and no use was recorded by the asking")
	assert.Equal(t, at, when)
}

// The vocabulary is the check: a scope outside it is refused, so the list
// the view reports and the list the store accepts cannot drift apart.
func TestTheReportedScopesAreTheOnesTheStoreAccepts(t *testing.T) {
	store, err := Open("")
	require.NoError(t, err)

	for _, scope := range Observe(store).Vocabulary {
		require.NoError(t, store.Record(scope, "item"), scope)
	}
	assert.Error(t, store.Record("plans", "item"), "and nothing outside it is recorded")

	facts := Observe(store)
	assert.Equal(t, len(Scopes()), facts.Items)
	facts.Vocabulary[0] = "mutated"
	assert.Equal(t, SlashCommands, Scopes()[0], "the caller gets a copy of the order, not the order")
}
