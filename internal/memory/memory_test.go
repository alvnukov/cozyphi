package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

const feedbackFile = `---
name: table-driven-tests
description: The user wants new tests written table-driven.
metadata:
  type: feedback
---

Write new tests table-driven, one case struct per row.

**Why:** every existing suite reads that way.
**How to apply:** see [[test-layout]] for where the file goes.
`

func TestParseFileReadsFrontmatterAndBody(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "table-driven-tests.md", feedbackFile)

	entry, err := ParseFile(path)
	require.NoError(t, err)
	assert.Equal(t, "table-driven-tests", entry.Name)
	assert.Equal(t, "The user wants new tests written table-driven.", entry.Description)
	assert.Equal(t, KindFeedback, entry.Kind)
	assert.Equal(t, "table-driven-tests.md", entry.File)
	assert.Equal(t, path, entry.Path)
	assert.Contains(t, entry.Body, "one case struct per row")
	assert.NotContains(t, entry.Body, "description:")
	assert.Equal(t, []string{"test-layout"}, entry.Links)
}

func TestParseFileReadsClaudeTopicMetadata(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "commit-each-stage.md", `---
name: commit-each-stage
description: Commit every completed stage of work.
metadata:
  node_type: memory
  type: feedback
  originSessionId: 7e00ccbd
  modified: 2026-08-27T08:00:00Z
---
Commit each stage after its checks pass.
`)

	entry, err := ParseFile(path)
	require.NoError(t, err)
	assert.Equal(t, "commit-each-stage", entry.Name)
	assert.Equal(t, KindFeedback, entry.Kind)
	assert.Equal(t, "Commit each stage after its checks pass.", entry.Body)
}

func TestParseFileAcceptsFlatTypeAndQuotedValues(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "dashboard.md", `---
name: "grafana-dashboard"
description: 'Latency board for the API.'
type: reference
---
https://grafana.example/d/api
`)

	entry, err := ParseFile(path)
	require.NoError(t, err)
	assert.Equal(t, "grafana-dashboard", entry.Name)
	assert.Equal(t, "Latency board for the API.", entry.Description)
	assert.Equal(t, KindReference, entry.Kind)
}

func TestParseFileFallsBackToFileNameAndProjectKind(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "release-freeze.md", `---
description: No releases until 2026-09-15.
metadata:
  type: nonsense
---
Ship nothing until the freeze lifts.
`)

	entry, err := ParseFile(path)
	require.NoError(t, err)
	assert.Equal(t, "release-freeze", entry.Name)
	assert.Equal(t, KindProject, entry.Kind, "an unknown type is kept as a fact, not dropped")
}

func TestParseFileRejectsNonMemoryFiles(t *testing.T) {
	dir := t.TempDir()

	_, err := ParseFile(write(t, dir, "notes.md", "just some notes\n"))
	assert.ErrorIs(t, err, ErrNoFrontmatter)

	_, err = ParseFile(write(t, dir, "half.md", "---\nname: half\ndescription: unfinished\n"))
	assert.ErrorIs(t, err, ErrOpenFrontmatter)
}

func TestOpenCreatesDirectoryAndIndex(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory", "--proj--")

	store, err := Open(dir, nil)
	require.NoError(t, err)
	assert.Equal(t, dir, store.Dir())

	index, err := os.ReadFile(filepath.Join(dir, IndexFile))
	require.NoError(t, err)
	assert.Empty(t, index, "an empty Claude memory has an empty catalog")
}

func TestSyncIndexWritesClaudeCompatibleCatalogAndReportsChange(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir, nil)
	require.NoError(t, err)

	write(t, dir, "table-driven-tests.md", feedbackFile)
	write(t, dir, "who-the-user-is.md", `---
name: who-the-user-is
description: Go developer, owns the harness.
metadata:
  type: user
---
Works on cozyphi daily.
`)

	changed, err := store.SyncIndex()
	require.NoError(t, err)
	assert.True(t, changed)

	index, err := os.ReadFile(filepath.Join(dir, IndexFile))
	require.NoError(t, err)
	assert.Equal(t, "- [Who the user is](who-the-user-is.md) — Go developer, owns the harness.\n"+
		"- [Table driven tests](table-driven-tests.md) — The user wants new tests written table-driven.\n", string(index))

	changed, err = store.SyncIndex()
	require.NoError(t, err)
	assert.False(t, changed, "an unchanged directory must not rewrite the index")
}

func TestEntriesSkipsIndexAndUnreadableFiles(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir, nil)
	require.NoError(t, err)

	write(t, dir, "table-driven-tests.md", feedbackFile)
	write(t, dir, "scratch.txt", "not markdown")
	write(t, dir, "broken.md", "no frontmatter here\n")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "nested.md"), 0o755))

	entries := store.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "table-driven-tests", entries[0].Name)
}

func TestPromptBlockCarriesProtocolAndFacts(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir, nil)
	require.NoError(t, err)

	assert.Contains(t, store.PromptBlock(), "None saved yet.")

	write(t, dir, "table-driven-tests.md", feedbackFile)
	block := store.PromptBlock()
	assert.Contains(t, block, dir)
	assert.Contains(t, block, "metadata:")
	assert.Contains(t, block, "You share Claude Code's auto memory")
	assert.Contains(t, block, "same MEMORY.md catalog and topic files")
	assert.Contains(t, block, `<memory name="table-driven-tests" type="feedback" file="table-driven-tests.md">`)
	assert.Contains(t, block, "The user wants new tests written table-driven.")
	assert.Contains(t, block, "one case struct per row", "under budget the fact itself rides along")
}

func TestNilStoreIsInert(t *testing.T) {
	var store *Store
	assert.Empty(t, store.Dir())
	assert.Empty(t, store.Entries())
	assert.Empty(t, store.PromptBlock())
	changed, err := store.SyncIndex()
	assert.NoError(t, err)
	assert.False(t, changed)
	assert.Empty(t, store.Turn().Reminder(Query{Prompt: "anything"}))
}

func TestBudgetReportsWhatThePromptCarries(t *testing.T) {
	store := storeWith(t, map[string]string{
		"compaction-summary-ux.md": compactionMemory,
		"permission-prompts.md":    permissionsMemory,
	})

	budget := store.Budget()
	assert.Equal(t, 2, budget.Facts)
	assert.Equal(t, 1, budget.Standing, "the feedback memory is in force on every request")
	assert.Equal(t, 1, budget.Listed, "the project memory is named, and recalled when it matches")
	assert.Equal(t, len([]rune(store.PromptBlock())), budget.Runes, "the report measures the real block")
}

// TestPromptStaysBoundedWhateverTheDirectoryHolds pins the property that keeps
// memory from quietly eating the context: neither the facts nor the list that
// replaces them may grow without limit.
func TestPromptStaysBoundedWhateverTheDirectoryHolds(t *testing.T) {
	files := make(map[string]string, 200)
	for i := range 200 {
		name := fmt.Sprintf("note-%03d", i)
		files[name+".md"] = fmt.Sprintf("---\nname: %s\ndescription: A note about the %d-th thing "+
			"that happened here.\nmetadata:\n  type: project\n---\nBody of note %d.\n", name, i, i)
	}
	store := storeWith(t, files)

	budget := store.Budget()
	assert.Equal(t, 200, budget.Facts)
	assert.Zero(t, budget.Standing, "project memories are never in force by default")
	assert.Less(t, budget.Listed, budget.Facts, "and the list itself is cut")
	assert.Less(t, budget.Runes, 6000, "the whole block stays bounded")

	block := store.PromptBlock()
	assert.Equal(t, budget.Listed, strings.Count(block, "(project) — A note about"),
		"the block names exactly what the report claims")
	assert.Contains(t, block, "MEMORY.md in that directory indexes", "the rest is one read away")
}
