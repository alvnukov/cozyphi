package memorytool_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/memory"
	"github.com/alvnukov/cozyphi/internal/tools/memorytool"
)

const (
	hashlineFile = `---
name: hashline-edits
description: Edits anchor on a hashline tag, never a whole-file rewrite.
metadata:
  type: feedback
---
Set the hash from the 4 hex chars after # in the file header.

**Why:** stale anchors must fail closed. See [[edit-tool-contract]].
`
	freezeFile = `---
name: release-freeze
description: No releases until 2026-09-15.
metadata:
  type: project
---
Ship nothing until the freeze lifts.
`
)

func testStore(t *testing.T) *memory.Store {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"hashline-edits.md": hashlineFile,
		"release-freeze.md": freezeFile,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	store, err := memory.Open(dir, nil)
	require.NoError(t, err)
	return store
}

func run(t *testing.T, store *memory.Store, args string) (string, string) {
	t.Helper()
	tool := memorytool.Tool(store)
	require.Equal(t, "memory", tool.Definition.Name)
	res, err := tool.Run(t.Context(), json.RawMessage(args))
	require.NoError(t, err)
	return res.Content, res.Detail
}

func TestListNamesEverythingStored(t *testing.T) {
	content, detail := run(t, testStore(t), `{}`)

	assert.Contains(t, content, "2 of 2 memories:")
	assert.Contains(
		t,
		content,
		"- hashline-edits (feedback) — Edits anchor on a hashline tag, never a whole-file rewrite.",
	)
	assert.Contains(t, content, "- release-freeze (project) — No releases until 2026-09-15.")
	assert.Contains(t, content, "action=read")
	assert.Equal(t, "list (2)", detail)
}

func TestListRanksAgainstAQuery(t *testing.T) {
	content, _ := run(t, testStore(t), `{"action":"list","query":"when does the release freeze lift"}`)

	assert.Contains(t, content, `best match first for "when does the release freeze lift"`)
	assert.Contains(t, content, "release-freeze")
	assert.NotContains(t, content, "hashline-edits", "a memory the query does not name stays out")
}

func TestListSaysSoWhenNothingMatches(t *testing.T) {
	content, detail := run(t, testStore(t), `{"query":"kubernetes ingress annotations"}`)

	assert.Contains(t, content, "No memory matches")
	assert.Contains(t, content, "2 stored")
	assert.Equal(t, "no match", detail)
}

func TestReadReturnsOneMemoryInFull(t *testing.T) {
	store := testStore(t)

	content, detail := run(t, store, `{"action":"read","name":"hashline-edits"}`)
	assert.Contains(t, content, "hashline-edits.md")
	assert.Contains(t, content, "metadata:", "the frontmatter comes with it")
	assert.Contains(t, content, "[[edit-tool-contract]]", "and so do its links")
	assert.Equal(t, "hashline-edits", detail)

	byFile, _ := run(t, store, `{"action":"read","name":"hashline-edits.md"}`)
	assert.Equal(t, content, byFile, "the file name names the same memory")
}

func TestBadArgumentsAreRejected(t *testing.T) {
	tool := memorytool.Tool(testStore(t))
	for name, args := range map[string]string{
		"unknown name":   `{"action":"read","name":"never-written"}`,
		"read no name":   `{"action":"read"}`,
		"unknown action": `{"action":"forget"}`,
		"unknown field":  `{"action":"list","depth":3}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := tool.Run(t.Context(), json.RawMessage(args))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "memory")
		})
	}
}

func TestNilStoreReportsAnEmptyMemory(t *testing.T) {
	tool := memorytool.Tool(nil)

	res, err := tool.Run(t.Context(), nil)
	require.NoError(t, err)
	assert.Contains(t, res.Content, "Nothing is stored")

	_, err = tool.Run(t.Context(), json.RawMessage(`{"action":"read","name":"anything"}`))
	require.Error(t, err)
}

func TestDetailFromArgsDescribesTheCall(t *testing.T) {
	tool := memorytool.Tool(testStore(t))

	assert.Equal(
		t,
		"read hashline-edits",
		tool.DetailFromArgs(json.RawMessage(`{"action":"read","name":"hashline-edits"}`)),
	)
	assert.Equal(t, "list gate", tool.DetailFromArgs(json.RawMessage(`{"query":"gate"}`)))
	assert.Equal(t, "list", tool.DetailFromArgs(json.RawMessage(`{}`)))
	assert.Empty(t, tool.DetailFromArgs(json.RawMessage(`{"action":"forget"}`)))
}

func TestForgetArchivesAMemory(t *testing.T) {
	store := testStore(t)

	content, detail := run(t, store, `{"action":"forget","name":"release-freeze"}`)
	assert.Contains(t, content, "forgotten/release-freeze.md")
	assert.Contains(t, content, "not off the disk")
	assert.Equal(t, "forget release-freeze", detail)

	listed, _ := run(t, store, `{}`)
	assert.Contains(t, listed, "1 of 1 memories:")
	assert.NotContains(t, listed, "release-freeze")
}

func TestForgetRefusesAPinnedMemory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pinned.md"), []byte(
		"---\nname: pinned\ndescription: Must stay.\npin: true\nmetadata:\n  type: project\n---\nBody.\n"), 0o600))
	store, err := memory.Open(dir, nil)
	require.NoError(t, err)

	_, err = memorytool.Tool(store).Run(t.Context(), json.RawMessage(`{"action":"forget","name":"pinned"}`))
	require.ErrorContains(t, err, "pinned")
}

func TestOverlapsReportsMergeCandidates(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"release-freeze": "Ship nothing until the release freeze lifts.",
		"freeze-window":  "Ship nothing until the release freeze lifts, said twice.",
	} {
		file := "---\nname: " + name + "\ndescription: No releases until the freeze lifts on 2026-09-15.\n" +
			"metadata:\n  type: project\n---\n" + body + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, name+".md"), []byte(file), 0o600))
	}
	store, err := memory.Open(dir, nil)
	require.NoError(t, err)

	content, detail := run(t, store, `{"action":"overlaps"}`)
	assert.Contains(t, content, "release-freeze")
	assert.Contains(t, content, "freeze-window")
	assert.Contains(t, content, "then forgetting the")
	assert.Equal(t, "overlaps (1)", detail)

	empty, _ := run(t, testStore(t), `{"action":"overlaps"}`)
	assert.Contains(t, empty, "No two memories overlap")
}
