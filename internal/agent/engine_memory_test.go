package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/memory"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// recordingServer answers every request with plain text and keeps the request
// bodies, which is where the system prompt and the user turn show up.
func recordingServer(t *testing.T, respond func(n int, w http.ResponseWriter)) (*httptest.Server, func() []string) {
	t.Helper()
	var (
		mu     sync.Mutex
		bodies []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		n := len(bodies)
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		respond(n, w)
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), bodies...)
	}
}

// writeMemory drops one fact file into a memory directory.
func writeMemory(t *testing.T, dir, kind, name, description, body string) {
	t.Helper()
	file := fmt.Sprintf("---\nname: %s\ndescription: %s\nmetadata:\n  type: %s\n---\n%s\n",
		name, description, kind, body)
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".md"), []byte(file), 0o600))
}

func drain(t *testing.T, engine *Engine, prompt string) {
	t.Helper()
	for _, err := range engine.Loop(t.Context(), prompt, LoopOpts{}) {
		require.NoError(t, err)
	}
}

// TestLoopCarriesMemoryIntoTheTurn pins the small-directory path: memory that
// fits the budget travels in the system prompt in full, and no recall block is
// prepended to the user's text.
func TestLoopCarriesMemoryIntoTheTurn(t *testing.T) {
	dir := t.TempDir()
	writeMemory(t, dir, "feedback", "hashline-edits", "Edits anchor on a hashline tag.",
		"Never swap hashline edits for a whole-file rewrite.")
	store, err := memory.Open(dir, nil)
	require.NoError(t, err)

	server, bodies := recordingServer(t, func(int, http.ResponseWriter) {})
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Gate:        permission.AllowAll{},
		Memory:      store,
	})
	require.NoError(t, err)

	drain(t, engine, "why do hashline edits fail")

	require.NotEmpty(t, bodies())
	body := bodies()[0]
	assert.Contains(t, body, "# Memory", "the protocol block rides in the system prompt")
	assert.Contains(t, body, "hashline-edits")
	assert.Contains(t, body, "whole-file rewrite", "the fact itself rides with it")
	assert.NotContains(t, body, "system-reminder", "nothing to recall: the prompt already has it")
	assert.Contains(t, body, `"name":"memory"`, "and the tool that reads the rest is registered")
}

// TestLoopRecallsAProjectMemoryForTheTurn pins the retrieval half: a project
// memory is named in the prompt and arrives in full, as a system-reminder,
// only on a turn it matches.
func TestLoopRecallsAProjectMemoryForTheTurn(t *testing.T) {
	dir := t.TempDir()
	writeMemory(t, dir, "project", "hashline-anchors", "Hashline anchors go stale after an edit.",
		"Re-read the file before anchoring again.")
	for _, name := range []string{"alpha", "beta", "gamma"} {
		writeMemory(t, dir, "project", name+"-note", "A note about "+name+".", "Body of "+name+".")
	}
	store, err := memory.Open(dir, nil)
	require.NoError(t, err)

	server, bodies := recordingServer(t, func(int, http.ResponseWriter) {})
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Gate:        permission.AllowAll{},
		Memory:      store,
	})
	require.NoError(t, err)

	drain(t, engine, "why do hashline anchors go stale")

	require.NotEmpty(t, bodies())
	body := bodies()[0]
	assert.Contains(t, body, "hashline-anchors (project)", "the name rides in the system prompt")
	assert.Contains(t, body, "system-reminder")
	assert.Contains(t, body, "Re-read the file before anchoring again.", "the match arrives in full")
	assert.NotContains(t, body, "Body of alpha.", "an unrelated memory stays out")
}

func TestLoopWithoutMemoryStoreAddsNothing(t *testing.T) {
	server, bodies := recordingServer(t, func(int, http.ResponseWriter) {})
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Gate:        permission.AllowAll{},
	})
	require.NoError(t, err)

	drain(t, engine, "why do hashline edits fail")

	require.NotEmpty(t, bodies())
	assert.NotContains(t, bodies()[0], "# Memory")
	assert.NotContains(t, bodies()[0], "system-reminder")
	assert.NotContains(t, bodies()[0], `"name":"memory"`, "no store, no memory tool")
}

// TestLoopIndexesAMemoryWrittenDuringTheTurn pins the other half: a file the
// agent writes mid-turn is indexed when the turn ends, and the next turn's
// system prompt carries it.
func TestLoopIndexesAMemoryWrittenDuringTheTurn(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.Open(dir, nil)
	require.NoError(t, err)

	server, bodies := recordingServer(t, func(n int, w http.ResponseWriter) {
		if n == 1 {
			_, _ = fmt.Fprint(w, sseToolCallChunk("call_1", "remember", `{}`))
			return
		}
		_, _ = fmt.Fprint(w, sseTextChunk())
	})

	remember := tools.Tool{
		Definition: llm.ToolDefinition{
			Name:        "remember",
			Description: "write a memory file",
			Params:      &llm.FunctionParameters{Type: "object"},
		},
		Run: func(context.Context, json.RawMessage) (tools.Result, error) {
			err := os.WriteFile(filepath.Join(dir, "release-freeze.md"), []byte(`---
name: release-freeze
description: No releases until 2026-09-15.
metadata:
  type: project
---
Ship nothing until the freeze lifts.
`), 0o644)
			return tools.Result{Content: "saved"}, err
		},
	}

	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Gate:        permission.AllowAll{},
		Tools:       []tools.Tool{remember},
		Memory:      store,
	})
	require.NoError(t, err)

	drain(t, engine, "remember the freeze")

	index, err := os.ReadFile(filepath.Join(dir, memory.IndexFile))
	require.NoError(t, err)
	assert.Contains(t, string(index), "[Release freeze](release-freeze.md)")

	drain(t, engine, "anything else")
	sent := bodies()
	require.GreaterOrEqual(t, len(sent), 3)
	assert.Contains(t, sent[len(sent)-1], "release-freeze (project)",
		"and the next turn's prompt names it")
}

// TestLoopSeesAMemoryRewrittenInPlace pins the invalidation seam. Replacing a
// file's contents under the same name moves no directory mtime, so only the
// engine noticing that a tool named the memory directory makes the new text
// reach the next turn.
func TestLoopSeesAMemoryRewrittenInPlace(t *testing.T) {
	dir := t.TempDir()
	writeMemory(t, dir, "project", "release-freeze", "No releases until 2026-09-15.",
		"Ship nothing until the freeze lifts.")
	store, err := memory.Open(dir, nil)
	require.NoError(t, err)

	path := filepath.Join(dir, "release-freeze.md")
	server, bodies := recordingServer(t, func(n int, w http.ResponseWriter) {
		if n == 1 {
			_, _ = fmt.Fprint(w, sseToolCallChunk("call_1", "rewrite", `{"path":`+strconv.Quote(path)+`}`))
			return
		}
		_, _ = fmt.Fprint(w, sseTextChunk())
	})

	rewrite := tools.Tool{
		Definition: llm.ToolDefinition{
			Name:        "rewrite",
			Description: "rewrite a memory file in place",
			Params:      &llm.FunctionParameters{Type: "object"},
		},
		Run: func(context.Context, json.RawMessage) (tools.Result, error) {
			file := "---\nname: release-freeze\ndescription: The freeze lifted on 2026-09-20.\n" +
				"metadata:\n  type: project\n---\nShipping is open again.\n"
			return tools.Result{Content: "saved"}, os.WriteFile(path, []byte(file), 0o600)
		},
	}

	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Gate:        permission.AllowAll{},
		Tools:       []tools.Tool{rewrite},
		Memory:      store,
	})
	require.NoError(t, err)

	drain(t, engine, "the freeze lifted, update the note")
	drain(t, engine, "anything else")

	sent := bodies()
	require.GreaterOrEqual(t, len(sent), 3)
	assert.Contains(t, sent[len(sent)-1], "The freeze lifted on 2026-09-20.")
	assert.NotContains(t, sent[len(sent)-1], "No releases until 2026-09-15.")
}

// TestCallNamesDirMatchesWindowsWirePaths pins the invalidation seam against
// JSON's path escaping: on Windows a tool call's arguments carry the memory
// directory with every separator doubled, and the raw substring check sees
// nothing. The seam is pure string logic, so the Windows wire forms are fed
// directly instead of being built for the host OS.
func TestCallNamesDirMatchesWindowsWirePaths(t *testing.T) {
	const dir = `C:\Users\runner\AppData\Local\Temp\memory`

	tests := []struct {
		name string
		args string
		want bool
	}{
		{
			name: "verbatim unix path",
			args: `{"path":"/tmp/memory/release-freeze.md"}`,
			want: false,
		},
		{
			name: "windows path with doubled separators",
			args: `{"path":"C:\\Users\\runner\\AppData\\Local\\Temp\\memory\\release-freeze.md"}`,
			want: true,
		},
		{
			name: "decoded windows path under a file key",
			args: `{"file":"C:\\Users\\runner\\AppData\\Local\\Temp\\memory\\notes.md"}`,
			want: true,
		},
		{
			name: "dir itself as the value",
			args: `{"path":"C:\\Users\\runner\\AppData\\Local\\Temp\\memory"}`,
			want: true,
		},
		{
			name: "lookalike outside the directory",
			args: `{"path":"C:\\Users\\runner\\AppData\\Local\\Temp\\memory-old\\notes.md"}`,
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, callNamesDir(tc.args, dir))
		})
	}
}
