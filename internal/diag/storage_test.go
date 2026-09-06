package diag_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// storageHome is the home directory the anchors below sit under. The
// registry collapses it to ~ on the way out, and every locator here is
// asserted in that collapsed spelling — the one a reader actually sees.
const storageHome = "/home/dev"

// liveStorage is the arrangement worth telling apart: a session persisting
// into this workspace's own directory, a corpus keyed by a checkout this
// workspace is only a worktree of, a registry the helper's config moved, and
// a shared history with something in every scope that matters.
func liveStorage() diag.StorageDeps {
	return diag.StorageDeps{
		Anchors:  liveAnchors,
		Sessions: liveSessions,
		Memory:   liveMemory,
		Tasks:    liveTasks,
		Usage:    liveUsage,
	}
}

func liveAnchors() diag.StorageAnchors {
	return diag.StorageAnchors{
		Known:         true,
		SessionBase:   "/home/dev/.cozyphi/session",
		MemoryBase:    "/home/dev/.claude/projects",
		UsageDir:      "/home/dev/.cozyphi",
		UsageFile:     "/home/dev/.cozyphi/usage.json",
		CorpusShared:  true,
		CorpusForeign: true,
	}
}

func liveSessions() diag.SessionStoreFacts {
	return diag.SessionStoreFacts{
		Known: true, Persisting: true, Bound: true, Written: true, Local: true,
		Entries: 42, Revision: "e42.btrue.wtrue",
	}
}

func liveMemory() diag.MemoryStoreFacts {
	return diag.MemoryStoreFacts{
		Known: true, Attempted: true, Opened: true, Indexed: true,
		Count: 6, Pinned: 2,
		Kinds: []diag.MemoryKindFacts{
			{Kind: "user", Count: 1},
			{Kind: "feedback", Count: 3},
			{Kind: "project", Count: 2},
			{Kind: "reference", Count: 0},
		},
		Vocabulary:     []string{"user", "feedback", "project", "reference"},
		VerifyInterval: 30 * time.Second,
		Revision:       "d6.p2.ifalse",
	}
}

func liveTasks() diag.TaskStoreFacts {
	return diag.TaskStoreFacts{
		Known: true, Attempted: true, Found: true,
		Dir: "notes/tasks", DefaultDir: "obsidian-tasks", Revision: "ffalse.dtrue",
	}
}

func liveUsage() diag.UsageStoreFacts {
	return diag.UsageStoreFacts{
		Known: true, Persistent: true,
		Scopes: []diag.UsageScopeFacts{
			{Scope: "slash_commands", Items: 4},
			{Scope: "models", Items: 12},
			{Scope: "model_efforts", Items: 0},
			{Scope: "skills", Items: 0},
			{Scope: "palette", Items: 3},
			{Scope: "memories", Items: 6},
		},
		Vocabulary: []string{"slash_commands", "models", "model_efforts", "skills", "palette", "memories"},
		Items:      25,
		Revision:   "i25.ffalse",
	}
}

// storageRegistry builds the registry with a home the anchors sit under, so
// the collapse the bounder performs is part of what these tests assert
// rather than something that depends on whose machine they run on.
func storageRegistry(t *testing.T, deps diag.StorageDeps) *diag.Registry {
	t.Helper()
	t.Setenv("HOME", storageHome)
	t.Setenv("USERPROFILE", storageHome)
	return diag.NewRegistry(fixedClock(), diag.DefaultLimits(), diag.NewStorageCollector(deps))
}

func storageFields(t *testing.T, deps diag.StorageDeps) []diag.Field {
	t.Helper()
	registry := storageRegistry(t, deps)
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryStorage)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	require.Equal(t, diag.AvailabilityAvailable, snapshot.Categories[0].Availability)
	require.False(t, snapshot.Truncated, "the category fits the response budget on its own")
	return snapshot.Categories[0].Fields
}

// A directory named after another path is a path in disguise: the home
// collapse works on separators, and there are none left inside an encoded
// segment. Neither the workspace's nor the checkout's encoded name may reach
// a locator, and the two questions they answer are answered once each
// elsewhere.
func TestNoLocatorSpellsOutADirectoryNamedAfterAnotherPath(t *testing.T) {
	fields := storageFields(t, liveStorage())

	transcript := fieldByKey(t, fields, diag.KeySessionsLocation)
	assert.Equal(t, "~/.cozyphi/session", transcript.Configured.Value.Str)
	assert.Equal(t, "~/.cozyphi/session/<workspace>/<session>.jsonl", transcript.Loaded.Value.Str)

	corpus := fieldByKey(t, fields, diag.KeyMemoryLocation)
	assert.Equal(t, "~/.claude/projects", corpus.Configured.Value.Str)
	assert.Equal(t, "~/.claude/projects/<corpus>/memory", corpus.Loaded.Value.Str)

	// Nothing else encodes a path, so the history is spelled out in full.
	history := fieldByKey(t, fields, diag.KeyUsageLocation)
	assert.Equal(t, "~/.cozyphi", history.Configured.Value.Str)
	assert.Equal(t, "~/.cozyphi/usage.json", history.Loaded.Value.Str)
}

// The anchors are the only paths this category may report, and each is
// reported in one of a handful of shapes. Anything else with a separator in
// it — a directory named after a working directory, a resumed transcript's
// path, a memory's own file — has no way out through here, and a field added
// later that grows one fails this.
func TestTheOnlyPathsThatTravelAreTheAnchorsAndTheirPlaceholders(t *testing.T) {
	allowed := []string{
		"~/.cozyphi/session", "~/.cozyphi/session/<workspace>/<session>.jsonl",
		"~/.claude/projects", "~/.claude/projects/<corpus>/memory",
		"~/.cozyphi", "~/.cozyphi/usage.json",
		"<repo>/obsidian-tasks", "<repo>/notes/tasks",
	}
	for _, field := range storageFields(t, liveStorage()) {
		for name, layer := range map[string]diag.Observation{
			"configured": field.Configured, "loaded": field.Loaded, "effective": field.Effective,
		} {
			for _, text := range append([]string{layer.Value.Str}, layer.Value.List...) {
				if !strings.ContainsRune(text, '/') {
					continue
				}
				assert.Contains(t, allowed, text, "%s %s", field.Key, name)
			}
		}
	}
}

// A transcript held is not a transcript written. A session takes its file
// when it opens and flushes on its first assistant turn, and the gap between
// the two is exactly what a reader mistakes for a lost conversation.
func TestABoundTranscriptWithNothingInItIsNotAnEphemeralSession(t *testing.T) {
	pending := liveSessions()
	pending.Written = false
	deps := liveStorage()
	deps.Sessions = func() diag.SessionStoreFacts { return pending }
	fields := storageFields(t, deps)

	state := fieldByKey(t, fields, diag.KeySessionsState)
	assert.Equal(t, string(diag.SessionsPersistent), state.Configured.Value.Str)
	assert.Equal(t, string(diag.SessionsFile), state.Loaded.Value.Str)
	assert.Equal(t, string(diag.SessionsPending), state.Effective.Value.Str)

	entries := fieldByKey(t, fields, diag.KeySessionsEntries)
	assert.Equal(t, int64(42), entries.Loaded.Value.Int, "the manager holds them either way")
	assert.Equal(t, diag.StateUnset, entries.Effective.State,
		"how much would survive this process is not zero, it is not yet a number")
	assert.NotEmpty(t, entries.Effective.Source.Ref)
}

// A session opened without persistence and one resumed from somewhere else
// are both "not in this workspace's directory", and they are not the same
// answer. Neither may report a locator it cannot honor.
func TestATranscriptKeptNowhereAndOneKeptElsewhereAnswerDifferently(t *testing.T) {
	ephemeral := diag.SessionStoreFacts{Known: true, Entries: 3}
	deps := liveStorage()
	deps.Sessions = func() diag.SessionStoreFacts { return ephemeral }
	unbound := fieldByKey(t, storageFields(t, deps), diag.KeySessionsLocation)
	assert.Equal(t, diag.StateUnset, unbound.Loaded.State)
	assert.Equal(t, string(diag.StorageIdentitySession), unbound.Effective.Value.Str)

	resumed := liveSessions()
	resumed.Local = false
	deps.Sessions = func() diag.SessionStoreFacts { return resumed }
	elsewhere := fieldByKey(t, storageFields(t, deps), diag.KeySessionsLocation)
	assert.Equal(t, diag.StateUnavailable, elsewhere.Loaded.State,
		"the path is arbitrary, and a locator that is not true is worse than none")
	assert.NotEmpty(t, elsewhere.Loaded.Source.Ref)
	assert.Equal(t, "~/.cozyphi/session", elsewhere.Configured.Value.Str,
		"where transcripts go is still where transcripts go")
}

// Two worktrees of one checkout share memories and do not share transcripts.
// That is the whole reason this category reports an identity beside every
// location: the path alone never says who else is writing there.
func TestEveryLocationSaysWhatItsStoreIsKeyedBy(t *testing.T) {
	fields := storageFields(t, liveStorage())

	for key, want := range map[string]diag.StorageIdentity{
		diag.KeySessionsLocation: diag.StorageIdentitySession,
		diag.KeyMemoryLocation:   diag.StorageIdentityRepository,
		diag.KeyTasksLocation:    diag.StorageIdentityRepository,
		diag.KeyUsageLocation:    diag.StorageIdentityUser,
	} {
		field := fieldByKey(t, fields, key)
		assert.Equal(t, string(want), field.Effective.Value.Str, key)
		assert.NotEmpty(t, field.Effective.Source.Ref, key)
	}
}

// A memory written in a linked worktree is one the main checkout reads. The
// corpus field is the only place that says so, and it says so three ways:
// what keys the corpus, how this workspace reaches it, and whether that is
// somebody else's directory.
func TestASessionInAWorktreeIsToldItSharesTheCheckoutsMemories(t *testing.T) {
	worktree := fieldByKey(t, storageFields(t, liveStorage()), diag.KeyMemoryCorpus)
	assert.Equal(t, string(diag.StorageIdentityRepository), worktree.Configured.Value.Str)
	assert.Equal(t, string(diag.MemoryCorpusWorktree), worktree.Loaded.Value.Str)
	assert.True(t, worktree.Effective.Value.Bool)

	deps := liveStorage()
	deps.Anchors = func() diag.StorageAnchors {
		anchors := liveAnchors()
		anchors.CorpusForeign = false
		return anchors
	}
	checkout := fieldByKey(t, storageFields(t, deps), diag.KeyMemoryCorpus)
	assert.Equal(t, string(diag.MemoryCorpusCheckout), checkout.Loaded.Value.Str)
	assert.False(t, checkout.Effective.Value.Bool)

	deps.Anchors = func() diag.StorageAnchors {
		anchors := liveAnchors()
		anchors.CorpusShared, anchors.CorpusForeign = false, false
		return anchors
	}
	own := fieldByKey(t, storageFields(t, deps), diag.KeyMemoryCorpus)
	assert.Equal(t, string(diag.StorageIdentityWorkspace), own.Configured.Value.Str)
	assert.Equal(t, string(diag.MemoryCorpusOwn), own.Loaded.Value.Str)
	assert.Equal(t, string(diag.StorageIdentityWorkspace),
		fieldByKey(t, storageFields(t, deps), diag.KeyMemoryLocation).Effective.Value.Str,
		"outside Git the working directory keys its own corpus")
}

// "No memories" is four situations with one symptom. Only one of them is a
// project that has not written any yet.
func TestTheMemoryLifecycleSeparatesTheWaysThereCanBeNoMemories(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state func(diag.MemoryStoreFacts) diag.MemoryStoreFacts
		want  diag.MemoryLifecycle
	}{
		{
			name: "nothing opened a corpus for this session",
			state: func(s diag.MemoryStoreFacts) diag.MemoryStoreFacts {
				return diag.MemoryStoreFacts{Known: true, Vocabulary: s.Vocabulary}
			},
			want: diag.MemoryNotAttempted,
		},
		{
			name: "the open failed",
			state: func(s diag.MemoryStoreFacts) diag.MemoryStoreFacts {
				return diag.MemoryStoreFacts{Known: true, Attempted: true, Vocabulary: s.Vocabulary}
			},
			want: diag.MemoryOpenFailed,
		},
		{
			name: "a store with no index built over it",
			state: func(s diag.MemoryStoreFacts) diag.MemoryStoreFacts {
				s.Indexed, s.Count, s.Pinned = false, 0, 0
				return s
			},
			want: diag.MemoryUnindexed,
		},
		{
			name: "an index over a directory with nothing in it",
			state: func(s diag.MemoryStoreFacts) diag.MemoryStoreFacts {
				s.Count, s.Pinned, s.Kinds = 0, 0, nil
				return s
			},
			want: diag.MemoryEmpty,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := liveStorage()
			deps.Memory = func() diag.MemoryStoreFacts { return tc.state(liveMemory()) }
			state := fieldByKey(t, storageFields(t, deps), diag.KeyMemoryState)
			assert.Equal(t, string(tc.want), state.Effective.Value.Str)
		})
	}
}

// A count nobody can take is not zero. An unbuilt index means the number
// would have to come from a read of the directory, and this view does not do
// one — so it says so instead of answering none.
func TestAnUnbuiltIndexReportsNoCountRatherThanACountOfNone(t *testing.T) {
	unindexed := liveMemory()
	unindexed.Indexed, unindexed.Count, unindexed.Pinned, unindexed.Kinds = false, 0, 0, nil
	deps := liveStorage()
	deps.Memory = func() diag.MemoryStoreFacts { return unindexed }
	fields := storageFields(t, deps)

	count := fieldByKey(t, fields, diag.KeyMemoryCount)
	assert.Equal(t, diag.StateUnavailable, count.Loaded.State)
	assert.Equal(t, diag.StateUnavailable, count.Effective.State)
	assert.NotEmpty(t, count.Loaded.Source.Ref)
	assert.Equal(t, diag.StateUnavailable, fieldByKey(t, fields, diag.KeyMemoryKinds).Loaded.State)

	// The one field that reports it rather than being silenced by it.
	index := fieldByKey(t, fields, diag.KeyMemoryIndex)
	assert.False(t, index.Loaded.Value.Bool)
	assert.False(t, index.Effective.Value.Bool)
	assert.Equal(t, "30s", index.Configured.Value.Str)
}

// The counts are of the index in force. A turn that wrote a memory marks it
// for re-checking, and until the next read the numbers above are what stands
// — which is worth reporting rather than leaving a reader to wonder.
func TestAnIndexMarkedForRecheckingIsBuiltButNotCurrent(t *testing.T) {
	stale := liveMemory()
	stale.Invalidated = true
	deps := liveStorage()
	deps.Memory = func() diag.MemoryStoreFacts { return stale }
	index := fieldByKey(t, storageFields(t, deps), diag.KeyMemoryIndex)

	assert.True(t, index.Loaded.Value.Bool, "an index is built")
	assert.False(t, index.Effective.Value.Bool, "and it is not the one a read would produce now")
}

// Kinds leave as a tally and a vocabulary. A kind with nothing in it is
// named in the vocabulary and left out of the tally, so the pair says which
// kinds are empty without a row of zeros.
func TestMemoryKindsLeaveAsCountsAgainstTheVocabulary(t *testing.T) {
	fields := storageFields(t, liveStorage())
	kinds := fieldByKey(t, fields, diag.KeyMemoryKinds)

	assert.Equal(t, []string{"user", "feedback", "project", "reference"}, kinds.Configured.Value.List)
	assert.Equal(t, []string{"user=1", "feedback=3", "project=2"}, kinds.Loaded.Value.List)
	assert.Equal(t, diag.StateNotApplicable, kinds.Effective.State)

	count := fieldByKey(t, fields, diag.KeyMemoryCount)
	assert.Equal(t, int64(6), count.Loaded.Value.Int, "the tally sums to the count")
	assert.Equal(t, int64(2), count.Effective.Value.Int)
}

// "No tasks" is four situations with one symptom, and the registry keeps no
// count at all — so the honest answer to how many there are is that it is
// not known here, never zero.
func TestTheTaskRegistryIsHonestAboutWhatItCannotCount(t *testing.T) {
	fields := storageFields(t, liveStorage())

	state := fieldByKey(t, fields, diag.KeyTasksState)
	assert.Equal(t, diag.StateNotApplicable, state.Configured.State)
	assert.Equal(t, string(diag.TasksDiscovered), state.Loaded.Value.Str)
	assert.True(t, state.Effective.Value.Bool)

	count := fieldByKey(t, fields, diag.KeyTasksCount)
	assert.Equal(t, diag.StateUnavailable, count.Loaded.State)
	assert.Equal(t, diag.StateUnavailable, count.Effective.State)
	assert.Contains(t, count.Loaded.Source.Ref, "reads the directory")

	for _, tc := range []struct {
		name  string
		facts diag.TaskStoreFacts
		want  diag.TasksLifecycle
	}{
		{"nobody looked", diag.TaskStoreFacts{Known: true}, diag.TasksNotAttempted},
		{
			"the repository has none",
			diag.TaskStoreFacts{Known: true, Attempted: true, DefaultDir: "obsidian-tasks"},
			diag.TasksAbsent,
		},
		{
			"the config could not be read",
			diag.TaskStoreFacts{Known: true, Attempted: true, Failed: true, DefaultDir: "obsidian-tasks"},
			diag.TasksDiscoverFailed,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := liveStorage()
			deps.Tasks = func() diag.TaskStoreFacts { return tc.facts }
			absent := storageFields(t, deps)
			assert.Equal(t, string(tc.want),
				fieldByKey(t, absent, diag.KeyTasksState).Loaded.Value.Str)
			assert.Equal(t, diag.StateNotApplicable,
				fieldByKey(t, absent, diag.KeyTasksCount).Loaded.State,
				"with no registry there is nothing to count rather than a count nobody took")
		})
	}
}

// The registry is tracked in the repository, so its address inside one is
// the whole of what a reader needs — and the default beside it is what says
// whether the helper's config moved it.
func TestTheTaskRegistryIsAddressedInsideItsOwnRepository(t *testing.T) {
	moved := fieldByKey(t, storageFields(t, liveStorage()), diag.KeyTasksLocation)
	assert.Equal(t, "<repo>/obsidian-tasks", moved.Configured.Value.Str)
	assert.Equal(t, "<repo>/notes/tasks", moved.Loaded.Value.Str)

	deps := liveStorage()
	deps.Tasks = func() diag.TaskStoreFacts {
		facts := liveTasks()
		facts.Found, facts.Dir = false, ""
		return facts
	}
	absent := fieldByKey(t, storageFields(t, deps), diag.KeyTasksLocation)
	assert.Equal(t, diag.StateUnset, absent.Loaded.State)
	assert.Equal(t, "<repo>/obsidian-tasks", absent.Configured.Value.Str,
		"where one would live is still an answer")
}

// A history that failed to parse looks exactly like a fresh install from
// every picker in the process. The middle layer is the only place that
// difference is recorded.
func TestAUsageHistoryThatFailedToLoadIsNotAFreshInstall(t *testing.T) {
	deps := liveStorage()
	deps.Usage = func() diag.UsageStoreFacts {
		facts := liveUsage()
		facts.OpenFailed, facts.Items, facts.Scopes = true, 0, nil
		return facts
	}
	failed := fieldByKey(t, storageFields(t, deps), diag.KeyUsageState)
	assert.Equal(t, string(diag.UsagePersistent), failed.Configured.Value.Str)
	assert.Equal(t, string(diag.UsageOpenFailed), failed.Loaded.Value.Str)
	assert.Equal(t, string(diag.UsageEmpty), failed.Effective.Value.Str)

	fresh := fieldByKey(t, storageFields(t, liveStorage()), diag.KeyUsageState)
	assert.Equal(t, string(diag.UsageOpen), fresh.Loaded.Value.Str)
	assert.Equal(t, string(diag.UsageLoaded), fresh.Effective.Value.Str)
}

// A history with no file behind it ranks this process's own choices and
// forgets them. It is stored nowhere, and saying so is not the same as
// failing to say where.
func TestAnInMemoryHistoryReportsNoFileRatherThanAnUnknownOne(t *testing.T) {
	deps := liveStorage()
	deps.Usage = func() diag.UsageStoreFacts {
		facts := liveUsage()
		facts.Persistent = false
		return facts
	}
	fields := storageFields(t, deps)
	assert.Equal(t, string(diag.UsageMemoryOnly),
		fieldByKey(t, fields, diag.KeyUsageState).Configured.Value.Str)
	location := fieldByKey(t, fields, diag.KeyUsageLocation)
	assert.Equal(t, diag.StateUnset, location.Loaded.State)
	assert.Equal(t, "~/.cozyphi", location.Configured.Value.Str)
}

// One of these scopes keys its items by the directory a memory corpus
// belongs to, and another by names a user typed. Counts are the only thing
// that may leave.
func TestUsageScopesLeaveAsCountsAndNeverAsKeys(t *testing.T) {
	scopes := fieldByKey(t, storageFields(t, liveStorage()), diag.KeyUsageScopes)

	assert.Equal(t, []string{"slash_commands", "models", "model_efforts", "skills", "palette", "memories"},
		scopes.Configured.Value.List)
	assert.Equal(t, []string{"slash_commands=4", "models=12", "palette=3", "memories=6"},
		scopes.Loaded.Value.List)
	assert.Equal(t, int64(25), scopes.Effective.Value.Int)
	for _, item := range scopes.Loaded.Value.List {
		assert.NotContains(t, item, "/", "a count carries no path and no key")
	}
}

// A wiring gap is not an empty store. Every field says it knows nothing
// rather than reporting a zero that would read as "nothing is kept".
func TestAnUnwiredStorageLayerReportsNothingRatherThanNone(t *testing.T) {
	registry := storageRegistry(t, diag.StorageDeps{})
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryStorage)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)

	require.NotEmpty(t, snapshot.Categories[0].Fields)
	for _, field := range snapshot.Categories[0].Fields {
		for name, layer := range map[string]diag.Observation{
			"configured": field.Configured, "loaded": field.Loaded, "effective": field.Effective,
		} {
			assert.Equal(t, diag.StateUnavailable, layer.State, "%s %s", field.Key, name)
			assert.Equal(t, diag.KindNone, layer.Value.Kind, "%s %s", field.Key, name)
		}
	}
}

// The catalog is answered from the key set alone, so listing it reaches no
// store — and every key it declares is one Explain will accept.
func TestTheStorageCatalogNamesExactlyWhatExplainAnswers(t *testing.T) {
	var reads int
	deps := liveStorage()
	sessions := deps.Sessions
	deps.Sessions = func() diag.SessionStoreFacts {
		reads++
		return sessions()
	}
	registry := storageRegistry(t, deps)

	catalog := registry.Catalog()
	var keys []string
	for _, entry := range catalog.Categories {
		if entry.Category == diag.CategoryStorage {
			keys = entry.Keys
			require.Equal(t, diag.AvailabilityAvailable, entry.Availability)
			assert.Contains(t, entry.Reason, "where this session's state is kept")
		}
	}
	require.Len(t, keys, 15)
	assert.Zero(t, reads, "listing the catalog reaches no owner")

	for _, key := range keys {
		explained, err := registry.Explain(t.Context(), diag.CategoryStorage, key)
		require.NoError(t, err, key)
		assert.Equal(t, key, explained.Field.Key)
	}
	_, err := registry.Explain(t.Context(), diag.CategoryStorage, "sessions.path")
	require.Error(t, err, "a key the catalog does not name is not answered")
}

// Every key is namespaced by its store, so the four of them can grow apart
// without colliding — and none of them collides with a key another category
// already answers.
func TestEveryStorageKeyIsNamespacedByItsStore(t *testing.T) {
	fields := storageFields(t, liveStorage())
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		require.False(t, seen[field.Key], "duplicate key %s", field.Key)
		seen[field.Key] = true
		prefix, _, ok := strings.Cut(field.Key, ".")
		require.True(t, ok, field.Key)
		assert.Contains(t, []string{"sessions", "memory", "tasks", "usage"}, prefix)
	}
}
