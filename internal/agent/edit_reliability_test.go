package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/tools/writetool"
	"github.com/alvnukov/cozyphi/internal/util"
)

// These deterministic provider trajectories test what actually reaches the
// model through Engine. They are protocol regressions, not model evaluations.
func TestEditReliabilityAcrossCompactionAndResume(t *testing.T) {
	work, transcripts := t.TempDir(), t.TempDir()
	path := filepath.Join(work, "settings.txt")
	original := "alpha\nbeta\ngamma"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
	makeEdit := func(content, from, to, replacement string) llm.Function {
		args, err := json.Marshal(writetool.EditInput{
			Path: path, Hash: util.ComputeFileHash(content),
			Edits: []writetool.FlatEdit{{From: from, To: to, Content: new(replacement)}},
		})
		require.NoError(t, err)
		return llm.Function{Name: "edit", Arguments: string(args)}
	}
	ref := func(line int, text string) string { return fmt.Sprintf("%d#%s", line, util.ComputeLineHash(text)) }
	readArgs, err := json.Marshal(map[string]string{"path": path, "mode": "edit"})
	require.NoError(t, err)
	read := llm.Function{Name: "read", Arguments: string(readArgs)}
	corrected := makeEdit(original, ref(2, "beta"), ref(2, "beta"), "BETA")
	shifted := makeEdit("alpha\nBETA\ngamma", ref(5, "gamma"), ref(5, "gamma"), "GAMMA")
	writeArgs, err := json.Marshal(map[string]string{"path": path, "content": "one\ntwo"})
	require.NoError(t, err)
	postWrite := makeEdit("one\ntwo", ref(2, "two"), ref(2, "two"), "TWO")
	resumedEdit := makeEdit("one\nTWO", ref(1, "one"), ref(1, "one"), "ONE")
	calls := []llm.Function{
		read,
		makeEdit(original, "bad", "bad", "BETA"),
		makeEdit(original, "bad", "bad", "BETA"), // unchanged retry remains a refusal
		corrected, shifted,
		{Name: "write", Arguments: string(writeArgs)},
		postWrite,
		{}, // end the first turn
		resumedEdit,
		{}, // same process after context compaction: capability survives
		makeEdit("ONE\nTWO", ref(1, "ONE"), ref(1, "ONE"), "final"), // fresh engine: no grant
		read,
		makeEdit("ONE\nTWO", ref(1, "ONE"), ref(1, "ONE"), "final"),
		{},
	}
	var mu sync.Mutex
	var outputs []string
	next := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []llm.Message `json:"messages"`
		}
		if decodeErr := json.NewDecoder(r.Body).Decode(&request); decodeErr != nil {
			t.Errorf("decode model request: %v", decodeErr)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if len(request.Messages) > 0 {
			last := request.Messages[len(request.Messages)-1]
			if last.Role == llm.RoleTool {
				outputs = append(outputs, last.Content)
			}
		}
		if next >= len(calls) {
			t.Error("engine requested an unexpected extra round")
			http.Error(w, "unexpected round", http.StatusInternalServerError)
			return
		}
		call := calls[next]
		next++
		w.Header().Set("Content-Type", "text/event-stream")
		chunk := sseTextChunk()
		if call.Name != "" {
			chunk = sseToolCallChunk(fmt.Sprintf("edit_eval_%d", next), call.Name, call.Arguments)
		}
		_, _ = fmt.Fprint(w, chunk+"data: [DONE]\n\n")
	}))
	defer server.Close()
	newEngine := func(resume string) *Engine {
		gate, gateErr := permission.NewGate(permission.Policy{
			WorkspaceOnlyReads: true, WorkspaceOnlyWrites: true, BashDefault: permission.Deny,
		}, work)
		require.NoError(t, gateErr)
		engine, engineErr := NewEngine(EngineOpts{
			Model: llm.ModelConfig{Name: "scripted-edit-trajectory", BaseURL: server.URL, APIKey: "test"},
			Tools: tools.DefaultTools(), Gate: gate, MaxRounds: 16,
			SessionOpts: SessionOpts{Cwd: work, SessionDir: transcripts, Persist: true, ResumePath: resume},
		})
		require.NoError(t, engineErr)
		return engine
	}
	run := func(engine *Engine) {
		for _, loopErr := range engine.Loop(t.Context(), "Apply the requested change.", LoopOpts{}) {
			require.NoError(t, loopErr)
		}
	}
	engine := newEngine("")
	run(engine)
	require.NoError(t, engine.Session().AppendCompaction(session.Compaction{Summary: "Continue the file edit."}))
	run(engine)
	run(newEngine(engine.SessionFile()))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "final\nTWO", string(got))
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, outputs, 11)
	require.True(t, strings.HasPrefix(outputs[1], "[edit:invalid_ref]"))
	require.Equal(t, outputs[1], outputs[2])
	require.Contains(t, outputs[3], "[edit:exact]")
	require.Contains(t, outputs[4], "[edit:rebased]")
	require.Contains(t, outputs[6], "[edit:exact]")
	require.Contains(t, outputs[7], "[edit:exact]")
	require.True(t, strings.HasPrefix(outputs[8], "[edit:no_capability]"))
	require.Contains(t, outputs[10], "[edit:exact]")
}
