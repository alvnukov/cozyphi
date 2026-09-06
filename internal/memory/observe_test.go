package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memoryFile is one memory of the given kind, with a body carrying the one
// string this package's observation must never let out.
func memoryFile(name, kind, secret string) string {
	return "---\nname: " + name + "\ndescription: a note about " + secret +
		"\nmetadata:\n  type: " + kind + "\n---\n\nThe key is " + secret + ".\n"
}

// corpus stands up a store over four memories, one of them pinned.
func corpus(t *testing.T, secret string) *Store {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "who-the-user-is.md", memoryFile("who-the-user-is", "user", secret))
	write(t, dir, "table-driven-tests.md", memoryFile("table-driven-tests", "feedback", secret))
	write(t, dir, "release-freeze.md", memoryFile("release-freeze", "project", secret))
	write(t, dir, "runbook.md", "---\nname: runbook\ndescription: where the runbook is\n"+
		"metadata:\n  type: reference\n  pin: true\n---\n\nSee the wiki.\n")
	store, err := Open(dir, nil)
	require.NoError(t, err)
	return store
}

// A memory's whole point is holding what someone told the agent once, so a
// corpus is the likeliest place in the process for a credential to sit. What
// leaves an observation is counts.
func TestNoMemorysNameDescriptionOrBodyReachesTheObservation(t *testing.T) {
	const secret = "sk-live-MEMORY-SENTINEL"
	store := corpus(t, secret)

	facts := Observe(store, ObserveOpen(nil))

	require.True(t, facts.Indexed)
	assert.Equal(t, 4, facts.Count)
	assert.Equal(t, 1, facts.Pinned)
	assert.Equal(t, []string{"user", "feedback", "project", "reference"}, facts.Vocabulary)
	assert.Len(t, facts.Kinds, 4)
	for _, kind := range facts.Kinds {
		assert.Equal(t, 1, kind.Count, kind.Kind)
	}

	require.Contains(t, store.Entries()[0].Body+store.Entries()[1].Body+
		store.Entries()[2].Body+store.Entries()[3].Body, secret,
		"the store still has it, which is why it is worth checking that the observation does not")
	assert.NotContains(t, printed(facts), secret)
	assert.NotContains(t, printed(facts), store.Dir(), "not even the directory it is kept in")
	assert.NotContains(t, printed(facts), "release-freeze")
}

// Asking must not be a read of the corpus: the index is what a turn scores
// against, and rebuilding it here would make a question about memories
// change which ones the next turn recalls.
func TestObservingAMemoryStoreBuildsNothingAndReadsNothing(t *testing.T) {
	store := corpus(t, "irrelevant")
	require.NotNil(t, store.index())

	before := Observe(store, ObserveOpen(nil))
	// A memory written outside cozyphi is exactly what a rebuild would pick
	// up, and what this observation must not.
	require.NoError(t, os.WriteFile(filepath.Join(store.Dir(), "written-behind-its-back.md"),
		[]byte(memoryFile("written-behind-its-back", "project", "nothing")), 0o644))

	for range 3 {
		assert.Equal(t, before, Observe(store, ObserveOpen(nil)),
			"the same answer every time, from the index already in memory")
	}
	assert.Equal(t, 4, before.Count, "and the new file is not in it")
}

// The counts are of the index in force, and a turn that wrote a memory marks
// it for re-checking. Saying so is the difference between a stale count and
// a count a reader would take for current.
func TestAnInvalidatedIndexIsReportedWithoutBeingRebuilt(t *testing.T) {
	store := corpus(t, "irrelevant")
	require.False(t, Observe(store, ObserveOpen(nil)).Invalidated)

	store.Invalidate()

	facts := Observe(store, ObserveOpen(nil))
	assert.True(t, facts.Invalidated)
	assert.True(t, facts.Indexed, "the index is kept; it is the re-check that is pending")
	assert.Equal(t, 4, facts.Count)
	assert.NotEqual(t, "d4.p1.ifalse", storeRevision(facts), "and the two states are told apart")
}

// Open hands back nil on failure, so a session that failed to open a corpus
// holds nothing to ask. The three answers that reach the view are carried
// beside the store instead — and none of them is "there are no memories".
func TestAFailedOpenIsNotAnEmptyCorpusAndNeitherIsNoOpenAtAll(t *testing.T) {
	for _, tc := range []struct {
		name      string
		store     *Store
		open      OpenFacts
		known     bool
		attempted bool
		opened    bool
	}{
		{name: "nobody opened one", known: false},
		{
			name: "the open failed",
			open: ObserveOpen(os.ErrPermission),
			// A failed open is a session that tried and holds nothing.
			known: true, attempted: true,
		},
		{
			name:  "a store is in force",
			store: corpus(t, "irrelevant"), open: ObserveOpen(nil),
			known: true, attempted: true, opened: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := Observe(tc.store, tc.open)
			assert.Equal(t, tc.known, facts.Known)
			assert.Equal(t, tc.attempted, facts.Attempted)
			assert.Equal(t, tc.opened, facts.Opened)
			assert.Equal(t, tc.opened, facts.Indexed)
			assert.Equal(t, vocabulary(), facts.Vocabulary,
				"what kinds exist is a property of the build, answerable with no store at all")
		})
	}
}

// The error a failed open returns names the directory and can quote the path
// it could not resolve. Only the fact of failure is kept.
func TestTheOpenErrorIsDroppedRatherThanCarried(t *testing.T) {
	const secret = "/private/var/folders/sk-live-PATH-SENTINEL"
	facts := ObserveOpen(&os.PathError{Op: "open", Path: secret, Err: os.ErrPermission})

	assert.True(t, facts.Failed)
	assert.NotContains(t, printed(facts), secret)
}

func printed(value any) string { return fmt.Sprintf("%+v", value) }
