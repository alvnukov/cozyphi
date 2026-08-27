package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeUsage is the harness side of the Usage seam, with the clock under the
// test's control.
type fakeUsage struct {
	count map[string]int
	last  map[string]time.Time
}

func newUsage() *fakeUsage {
	return &fakeUsage{count: make(map[string]int), last: make(map[string]time.Time)}
}

func (f *fakeUsage) Use(name string) {
	f.count[name]++
	f.last[name] = time.Now()
}

func (f *fakeUsage) Seen(name string) (int, time.Time) { return f.count[name], f.last[name] }

func (f *fakeUsage) usedAt(name string, count int, when time.Time) {
	f.count[name], f.last[name] = count, when
}

func storeUsing(t *testing.T, use Usage, files map[string]string) *Store {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		write(t, dir, name, content)
	}
	store, err := Open(dir, use)
	require.NoError(t, err)
	return store
}

// aged backdates a file, which is how a test says "this has been sitting here".
func aged(t *testing.T, store *Store, file string, age time.Duration) {
	t.Helper()
	when := time.Now().Add(-age)
	require.NoError(t, os.Chtimes(filepath.Join(store.Dir(), file), when, when))
	store.Invalidate()
}

func fact(name, kind, description, body string) string {
	return "---\nname: " + name + "\ndescription: " + description +
		"\nmetadata:\n  type: " + kind + "\n---\n" + body + "\n"
}

func TestForgetMovesAMemoryOutOfTheWayRatherThanDeletingIt(t *testing.T) {
	store := storeWith(t, map[string]string{"compaction-summary-ux.md": compactionMemory})

	entry, err := store.Forget("compaction-summary-ux")
	require.NoError(t, err)
	assert.Equal(t, "compaction-summary-ux", entry.Name)

	assert.Empty(t, store.Entries(), "it leaves the index")
	assert.Empty(t, store.Turn().Reminder(Query{Prompt: "the compaction summary row"}), "and retrieval")
	assert.FileExists(t, filepath.Join(store.Dir(), forgottenDir, "compaction-summary-ux.md"),
		"but not the disk: a wrong call has to be undoable")

	forgotten := store.Forgotten()
	require.Len(t, forgotten, 1)
	assert.Equal(t, "compaction-summary-ux", forgotten[0].Name)
}

func TestForgetRefusesWhatIsPinnedOrUnknown(t *testing.T) {
	store := storeWith(t, map[string]string{
		"release-freeze.md": "---\nname: release-freeze\ndescription: No releases until 2026-09-15.\n" +
			"pin: true\nmetadata:\n  type: project\n---\nShip nothing until the freeze lifts.\n",
	})

	_, err := store.Forget("release-freeze")
	require.ErrorContains(t, err, "pinned")
	assert.FileExists(t, filepath.Join(store.Dir(), "release-freeze.md"))

	_, err = store.Forget("never-written")
	require.ErrorContains(t, err, "no memory named")
}

func TestCompactArchivesAnExactDuplicate(t *testing.T) {
	body := "The summary row stays collapsed until the user expands it."
	store := storeWith(t, map[string]string{
		"collapsed-row.md": fact("collapsed-row", "project", "Compaction rows render collapsed.", body),
		"summary-row.md":   fact("summary-row", "project", "Compaction rows render collapsed.", body),
		"unrelated-note.md": fact(
			"unrelated-note",
			"project",
			"The footer shows token labels.",
			"Nothing to do with rows.",
		),
	})
	aged(t, store, "collapsed-row.md", 48*time.Hour)

	archived := store.Compact()

	assert.Equal(t, []string{"collapsed-row"}, archived, "the older copy goes, the newer stays")
	names := make([]string, 0, 2)
	for _, entry := range store.Entries() {
		names = append(names, entry.Name)
	}
	assert.ElementsMatch(t, []string{"summary-row", "unrelated-note"}, names)
}

func TestCompactLeavesADuplicateSomethingLinksTo(t *testing.T) {
	body := "The summary row stays collapsed until the user expands it."
	store := storeWith(t, map[string]string{
		"collapsed-row.md": fact("collapsed-row", "project", "Compaction rows render collapsed.", body),
		"summary-row.md":   fact("summary-row", "project", "Compaction rows render collapsed.", body),
		"transcript.md":    fact("transcript", "project", "How the transcript renders.", "See [[collapsed-row]]."),
	})
	aged(t, store, "collapsed-row.md", 48*time.Hour)

	assert.Empty(t, store.Compact(), "the name is part of the fact once something points at it")
	assert.Len(t, store.Entries(), 3)
}

func TestStaleNeedsAgeAndDisuseTogether(t *testing.T) {
	use := newUsage()
	store := storeUsing(t, use, map[string]string{
		"old-unused.md":   fact("old-unused", "project", "A note nobody needed again.", "Body."),
		"old-but-used.md": fact("old-but-used", "project", "A note that keeps coming up.", "Body."),
		"old-and-pinned.md": "---\nname: old-and-pinned\ndescription: The one that must not go.\n" +
			"pin: true\nmetadata:\n  type: project\n---\nBody.\n",
		"written-today.md": fact("written-today", "project", "Learned an hour ago.", "Body."),
	})
	for _, file := range []string{"old-unused.md", "old-but-used.md", "old-and-pinned.md"} {
		aged(t, store, file, 200*24*time.Hour)
	}
	use.usedAt("old-but-used", 4, time.Now().Add(-24*time.Hour))

	stale := store.Stale()

	require.Len(t, stale, 1, "age alone is not disuse, and a pin is never stale")
	assert.Equal(t, "old-unused", stale[0].Name)
}

func TestStaleIsEmptyWithoutUsageHistory(t *testing.T) {
	store := storeWith(t, map[string]string{"note.md": fact("note", "project", "A note.", "Body.")})
	aged(t, store, "note.md", 400*24*time.Hour)

	assert.Empty(t, store.Stale(), "with nothing recording use, nothing can be called unused")
}

func TestPromptKeepsWhatEarnsItsPlaceWhenTheBudgetRunsOut(t *testing.T) {
	use := newUsage()
	files := make(map[string]string, 6)
	for _, name := range []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta"} {
		files[name+"-rule.md"] = fact(name+"-rule", "feedback",
			"How the user wants "+name+" handled, at some length so the budget matters.",
			strings.Repeat("A sentence about "+name+" that takes up room. ", 4))
	}
	store := storeUsing(t, use, files)
	use.usedAt("zeta-rule", 9, time.Now().Add(-time.Hour))

	block := store.PromptBlock()

	assert.Contains(t, block, `name="zeta-rule"`, "what gets used keeps its place")
	assert.Less(t, store.Budget().Standing, 6, "and the budget really did cut")
	assert.Contains(t, block, "## Memory needs attention", "the cut is announced, not silent")
	assert.Contains(t, block, "no room in the prompt")
}

func TestPinSurvivesTheBudget(t *testing.T) {
	files := make(map[string]string, 6)
	for _, name := range []string{"alpha", "beta", "gamma", "delta", "epsilon"} {
		files[name+"-rule.md"] = fact(name+"-rule", "feedback",
			"How the user wants "+name+" handled, at some length so the budget matters.",
			strings.Repeat("A sentence about "+name+" that takes up room. ", 4))
	}
	files["never-drop.md"] = "---\nname: never-drop\ndescription: The one rule that must always be in force.\n" +
		"pin: true\nmetadata:\n  type: feedback\n---\n" +
		strings.Repeat("A sentence that takes up just as much room. ", 4) + "\n"
	store := storeUsing(t, newUsage(), files)

	assert.Contains(t, store.PromptBlock(), `name="never-drop"`, "a pin outranks the arithmetic")
}

func TestMaintenanceStaysQuietUntilThereIsPressure(t *testing.T) {
	store := storeUsing(t, newUsage(), map[string]string{
		"dry-tone.md":       fact("dry-tone", "user", "The user wants dry, concise answers.", "No preamble."),
		"release-freeze.md": fact("release-freeze", "project", "No releases until 2026-09-15.", "Ship nothing."),
	})

	assert.NotContains(t, store.PromptBlock(), "Memory needs attention",
		"two facts and nothing wrong is not a problem to report")
}

func TestOverlapsFindsWhatSaysTheSameThingTwice(t *testing.T) {
	store := storeWith(t, map[string]string{
		"release-freeze.md": fact("release-freeze", "project",
			"No releases until the freeze lifts on 2026-09-15.",
			"Ship nothing until the release freeze lifts."),
		"freeze-window.md": fact("freeze-window", "project",
			"No releases until the freeze lifts on 2026-09-15.",
			"Ship nothing until the release freeze lifts, said twice."),
		"footer-tokens.md": fact("footer-tokens", "project",
			"The footer shows token labels.", "It follows the terminal theme."),
	})

	pairs := store.Overlaps(0, 5)

	require.Len(t, pairs, 1)
	assert.ElementsMatch(t, []string{"release-freeze", "freeze-window"},
		[]string{pairs[0].A.Name, pairs[0].B.Name})
	assert.GreaterOrEqual(t, pairs[0].Similarity, overlapThreshold)
}
