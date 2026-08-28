package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRankBalancesFrequencyAndRecency(t *testing.T) {
	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	store := newStore("", func() time.Time { return now })

	store.entries = map[string]map[string]entry{
		Models: {
			"frequent-old": {Count: 4, LastUsed: now.Add(-365 * 24 * time.Hour)},
			"recent":       {Count: 1, LastUsed: now.Add(-time.Hour)},
			"frequent-new": {Count: 2, LastUsed: now.Add(-time.Hour)},
		},
	}

	got := Rank(store, Models, []string{"unused", "frequent-old", "recent", "frequent-new"}, func(item string) string {
		return item
	})

	assert.Equal(t, []string{"frequent-new", "recent", "frequent-old", "unused"}, got)
}

func TestRankPreservesInputOrderForNewItemsAndTies(t *testing.T) {
	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	store := newStore("", func() time.Time { return now })
	store.entries = map[string]map[string]entry{
		SlashCommands: {
			"second": {Count: 1, LastUsed: now},
			"third":  {Count: 1, LastUsed: now},
		},
	}
	input := []string{"first", "second", "third", "fourth"}

	got := Rank(store, SlashCommands, input, func(item string) string { return item })

	assert.Equal(t, []string{"second", "third", "first", "fourth"}, got)
	assert.Equal(t, []string{"first", "second", "third", "fourth"}, input, "ranking must not mutate picker input")
}

func TestStoreRecordPersistsAndReloadsUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	store := newStore(path, func() time.Time { return now })

	require.NoError(t, store.Record(Skills, "tdd"))
	now = now.Add(time.Hour)
	require.NoError(t, store.Record(Skills, "tdd"))

	reloaded, err := Open(path)
	require.NoError(t, err)
	entry := reloaded.entries[Skills]["tdd"]
	assert.Equal(t, uint64(2), entry.Count)
	assert.Equal(t, now, entry.LastUsed)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestOpenToleratesMissingAndUnknownItems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	store, err := Open(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"known", "new"}, Rank(store, Models, []string{"known", "new"}, func(item string) string {
		return item
	}))

	require.NoError(
		t,
		os.WriteFile(
			path,
			[]byte(`{"version":1,"entries":{"models":{"removed":{"count":5,"lastUsed":"2026-03-01T00:00:00Z"}}}}`),
			0o600,
		),
	)
	store, err = Open(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"known", "new"}, Rank(store, Models, []string{"known", "new"}, func(item string) string {
		return item
	}))
}

func TestOpenReturnsUsableEmptyStoreForMalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))

	store, err := Open(path)

	require.Error(t, err)
	require.NotNil(t, store)
	assert.NoError(t, store.Record(Models, "gpt"))
}

func TestRankDecaysFrequencyWithAge(t *testing.T) {
	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	store := newStore("", func() time.Time { return now })
	store.entries = map[string]map[string]entry{
		Models: {
			"same-history-older": {Count: 50, LastUsed: now.Add(-90 * 24 * time.Hour)},
			"same-history-newer": {Count: 50, LastUsed: now},
		},
	}

	got := Rank(
		store,
		Models,
		[]string{"never-used", "same-history-older", "same-history-newer"},
		func(s string) string {
			return s
		},
	)

	assert.Equal(t, []string{"same-history-newer", "same-history-older", "never-used"}, got)
}

func TestRecordPrunesStaleEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	store := newStore(path, func() time.Time { return now })
	store.entries = map[string]map[string]entry{
		Models: {
			"stale": {Count: 9, LastUsed: now.Add(-staleAfter - time.Hour)},
			"live":  {Count: 1, LastUsed: now.Add(-time.Hour)},
		},
	}

	require.NoError(t, store.Record(Models, "fresh"))

	reloaded, err := Open(path)
	require.NoError(t, err)
	assert.NotContains(t, reloaded.entries[Models], "stale")
	assert.Contains(t, reloaded.entries[Models], "live")
	assert.Contains(t, reloaded.entries[Models], "fresh")
}
