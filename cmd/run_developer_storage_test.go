package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/usage"
)

// headlessStorage runs one developer-mode round that asks what this run
// keeps, and returns the fixture, the decoded answer and its raw text. The
// prepare hook runs after the bootstrap and before the run, which is the
// only window in which a store can be seeded: the run opens every one of
// them itself.
func headlessStorage(t *testing.T, prepare func(*developerFixture)) (
	*developerFixture, map[string]harnessField, string,
) {
	t.Helper()
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"snapshot","category":"storage"}`)
		}
		return doneText()
	})
	if prepare != nil {
		prepare(fixture)
	}

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "what do you keep", maxRounds: 3, timeout: 20 * time.Second, developerMode: true,
	})
	require.Equal(t, ExitOK, exit)

	outputs := fixture.toolOutputs()
	require.Len(t, outputs, 1, "exactly one harness call, so exactly one answer")
	return fixture, harnessFields(t, outputs[0]), outputs[0]
}

// seedMemory writes one memory into the corpus this run will open.
func seedMemory(body string) func(*developerFixture) {
	return func(f *developerFixture) {
		dir := f.bs.Proj.MemoryDir()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "release-freeze.md"), []byte(body), 0o600); err != nil {
			panic(err)
		}
	}
}

// A headless run composes its locators from the same anchors a session does,
// and the segments that merely encode another path are filled in rather than
// spelled out. The home collapse cannot reach inside a directory named after
// a working directory, because the separators are already gone by then.
func TestHeadlessSaysWhereItsStoresAreWithoutSpellingOutAnEncodedPath(t *testing.T) {
	fixture, fields, answer := headlessStorage(t, nil)
	encoded := filepath.Base(filepath.Dir(fixture.bs.Proj.MemoryDir()))
	require.Contains(t, encoded, "-", "the layout does encode the workspace into that name")

	transcript := fields["sessions.location"]
	assert.Equal(t, "~/.cozyphi/session", transcript.Configured.Value.String)
	assert.Equal(t, "~/.cozyphi/session/<workspace>/<session>.jsonl", transcript.Loaded.Value.String)

	corpus := fields["memory.location"]
	assert.Equal(t, "~/.claude/projects", corpus.Configured.Value.String)
	assert.Equal(t, "~/.claude/projects/<corpus>/memory", corpus.Loaded.Value.String)

	assert.Equal(t, "~/.cozyphi/usage.json", fields["usage.location"].Loaded.Value.String,
		"nothing in this one encodes a path, so it is reported as it is")

	assert.NotContains(t, answer, encoded)
	assert.NotContains(t, answer, filepath.Base(fixture.bs.SessionDir))
	assert.NotContains(t, answer, fixture.bs.Cwd)
}

// A headless run persists its transcript, and by the time a tool runs the
// first assistant turn has already reached the file. The three layers still
// carry three answers, because what a run was opened for, what it holds and
// what has been written are three different questions.
func TestHeadlessReportsATranscriptItIsAlreadyWriting(t *testing.T) {
	_, fields, _ := headlessStorage(t, nil)

	state := fields["sessions.state"]
	assert.Equal(t, "persistent", state.Configured.Value.String, "the run was opened to be written")
	assert.Equal(t, "file", state.Loaded.Value.String)
	assert.Equal(t, "persisted", state.Effective.Value.String)

	entries := fields["sessions.entries"]
	assert.Positive(t, entries.Loaded.Value.Int, "the header and the turns behind this call")
	assert.Equal(t, "present", entries.Effective.State,
		"the transcript has been written, so how much would survive is a number")
	assert.Equal(t, entries.Loaded.Value.Int, entries.Effective.Value.Int)
}

// The run's own workspace is a temporary directory outside Git, so it keys
// its own corpus and the repository has no task registry. Both are answers
// rather than gaps, and the count of tasks in a registry that is not there
// is neither.
func TestHeadlessKeepsItsOwnMemoriesAndFindsNoTaskRegistry(t *testing.T) {
	_, fields, _ := headlessStorage(t, nil)

	corpus := fields["memory.corpus"]
	assert.Equal(t, "workspace", corpus.Configured.Value.String)
	assert.Equal(t, "own", corpus.Loaded.Value.String)
	assert.False(t, corpus.Effective.Value.Bool, "nothing else is reading these memories")

	registry := fields["tasks.state"]
	assert.Equal(t, "absent", registry.Loaded.Value.String,
		"a repository with no registry is not a discovery that failed")
	assert.False(t, registry.Effective.Value.Bool)
	assert.Equal(t, "<repo>/obsidian-tasks", fields["tasks.location"].Configured.Value.String)
	assert.Equal(t, "unset", fields["tasks.location"].Loaded.State)
	assert.Equal(t, "not_applicable", fields["tasks.count"].Loaded.State)
}

// The corpus this run opens is the one the collector counts, and a memory is
// the likeliest place in the process for a credential to sit. What leaves is
// counts: the memory is there, and neither its name nor its body is.
func TestHeadlessCountsItsCorpusWithoutNamingAnythingInIt(t *testing.T) {
	const sentinel = "sk-live-HEADLESS-STORAGE"
	_, fields, answer := headlessStorage(t, seedMemory("---\nname: release-freeze\n"+
		"description: the key is "+sentinel+"\nmetadata:\n  type: project\n---\n\n"+
		"The key is "+sentinel+".\n"))

	require.Equal(t, int64(1), fields["memory.count"].Loaded.Value.Int,
		"the corpus really does hold it, which is why the rest is worth checking")
	assert.Equal(t, int64(0), fields["memory.count"].Effective.Value.Int, "and none of it is pinned")
	assert.Equal(t, []string{"project=1"}, fields["memory.kinds"].Loaded.Value.List)
	assert.Equal(t, "indexed", fields["memory.state"].Effective.Value.String)

	assert.NotContains(t, answer, sentinel)
	assert.NotContains(t, answer, "release-freeze")
}

// A history that could not be parsed leaves every picker in the process
// ranking as if nothing had ever been used. That is exactly what a fresh
// install looks like, so the run says which of the two it is.
func TestHeadlessTellsAFailedHistoryApartFromAFreshOne(t *testing.T) {
	fresh, fields, _ := headlessStorage(t, nil)
	assert.Equal(t, "persistent", fields["usage.state"].Configured.Value.String)
	assert.Equal(t, "open", fields["usage.state"].Loaded.Value.String)
	assert.Equal(t, "empty", fields["usage.state"].Effective.Value.String)
	require.NoFileExists(t, fresh.bs.Proj.Global().UsageFile(),
		"asking about the history did not bring one into being")

	_, broken, _ := headlessStorage(t, func(f *developerFixture) {
		require.NoError(t, os.WriteFile(f.bs.Proj.Global().UsageFile(), []byte("{not json"), 0o600))
	})
	assert.Equal(t, "persistent", broken["usage.state"].Configured.Value.String)
	assert.Equal(t, "open_failed", broken["usage.state"].Loaded.Value.String)
	assert.Equal(t, "empty", broken["usage.state"].Effective.Value.String)
}

// The scopes are the ones this build records, and what each remembers leaves
// as a count. One of them keys its items by the directory a corpus belongs
// to, which is a path, and another by a name someone typed.
func TestHeadlessTalliesTheHistoryByScopeAndNeverByKey(t *testing.T) {
	const sentinel = "sk-live-HEADLESS-HISTORY"
	_, fields, answer := headlessStorage(t, func(f *developerFixture) {
		history, err := usage.Open(f.bs.Proj.Global().UsageFile())
		require.NoError(t, err)
		require.NoError(t, history.Record(usage.Models, sentinel))
	})

	assert.Equal(t, usage.Scopes(), fields["usage.scopes"].Configured.Value.List)
	assert.Contains(t, fields["usage.scopes"].Loaded.Value.List, "models=1")
	assert.Positive(t, fields["usage.scopes"].Effective.Value.Int)
	for _, entry := range fields["usage.scopes"].Loaded.Value.List {
		assert.Regexp(t, `^[a-z_]+=[0-9]+$`, entry, "a tally row is a scope and a count")
	}
	assert.NotContains(t, answer, sentinel)
}

// A key is a stable address, so the catalog names every one of them without
// observing anything: listing what can be asked opens no store, builds no
// index and reads no directory.
func TestTheCatalogDeclaresEveryStorageKey(t *testing.T) {
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"catalog"}`)
		}
		return doneText()
	})
	require.Equal(t, ExitOK, runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "what can you be asked", maxRounds: 3, timeout: 20 * time.Second, developerMode: true,
	}))

	outputs := fixture.toolOutputs()
	require.Len(t, outputs, 1)
	var catalog struct {
		Categories []struct {
			Category string   `json:"category"`
			Reason   string   `json:"reason"`
			Keys     []string `json:"keys"`
		} `json:"categories"`
	}
	require.NoError(t, json.Unmarshal([]byte(outputs[0]), &catalog))

	var storage struct {
		Reason string
		Keys   []string
	}
	for _, entry := range catalog.Categories {
		if entry.Category == "storage" {
			storage.Reason, storage.Keys = entry.Reason, entry.Keys
		}
	}
	assert.Equal(t, []string{
		"sessions.state", "sessions.location", "sessions.entries",
		"memory.state", "memory.location", "memory.corpus",
		"memory.count", "memory.kinds", "memory.index",
		"tasks.state", "tasks.location", "tasks.count",
		"usage.state", "usage.location", "usage.scopes",
	}, storage.Keys)
	assert.Contains(t, storage.Reason, "where this session's state is kept",
		"the category says what it answers before a call is spent asking")
}

// Whatever else this run holds, the answer is composed from states, counts
// and identities. The provider credential sits in the same process and in
// the same configuration the run was built from.
func TestTheStorageAnswerCarriesNoCredentialAndNoAbsolutePath(t *testing.T) {
	fixture, _, answer := headlessStorage(t, nil)

	assert.NotContains(t, answer, "test-key", "the provider credential never reaches an observation")
	assert.NotContains(t, strings.ToLower(answer), "authorization")

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.NotContains(t, answer, home, "every anchor below it is collapsed to ~")
	assert.NotContains(t, answer, fixture.bs.Proj.Root())
}
