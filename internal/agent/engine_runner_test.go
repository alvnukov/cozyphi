package agent_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

func textOnlySSEServer(reply string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunk := jsonMarshalDelta(reply)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
		_, _ = fmt.Fprint(
			w,
			"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n",
		)
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
}

func jsonMarshalDelta(content string) string {
	// Keep tiny and stable for tests; escape is enough for plain ASCII replies.
	content = strings.ReplaceAll(content, `\`, `\\`)
	content = strings.ReplaceAll(content, `"`, `\"`)
	content = strings.ReplaceAll(content, "\n", `\n`)
	return `{"choices":[{"delta":{"role":"assistant","content":"` + content + `"}}]}`
}

func TestEngineRunnerViaJobManager(t *testing.T) {
	srv := textOnlySSEServer("explored auth module; see internal/auth/login.go")
	defer srv.Close()

	jobsRoot := t.TempDir()
	workdir := t.TempDir()

	runner := agent.EngineRunner{
		Model: llm.ModelConfig{
			Name:    "fake",
			BaseURL: srv.URL,
			APIKey:  "x",
		},
		MaxRounds: 4,
	}
	mgr, err := job.New(job.Options{
		Root:   jobsRoot,
		Runner: runner,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })

	info, err := mgr.Spawn(t.Context(), job.SpawnRequest{
		Prompt:      "Look at auth",
		Description: "auth explore",
		ParentID:    "parent-sess-1",
		WorkDir:     workdir,
	})
	require.NoError(t, err)
	res, err := mgr.Wait(t.Context(), info.ID)
	require.NoError(t, err)
	assert.Equal(t, job.StatusCompleted, res.Info.Status)
	assert.Contains(t, res.Summary, "auth module")

	// result.md on disk
	data, err := os.ReadFile(res.Info.ResultPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "login.go")

	// child session persisted under job dir
	sessDir := filepath.Join(res.Info.Dir, "session")
	entries, err := os.ReadDir(sessDir)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	var jsonl string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			jsonl = filepath.Join(sessDir, e.Name())
			break
		}
	}
	require.NotEmpty(t, jsonl)
	raw, err := os.ReadFile(jsonl)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "parent-sess-1")
	assert.Contains(t, string(raw), "parentSession")

	_, err = session.OpenSession(jsonl)
	require.NoError(t, err)
}

// TestEngineRunnerRejectsEscapingWorkdir pins the consumption side of spawn
// confinement: even a job whose persisted meta claims a workdir outside the
// parent workspace (a rogue or hand-edited Spawn caller) must fail closed
// before any engine — and thus any child gate — is assembled.
func TestEngineRunnerRejectsEscapingWorkdir(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()

	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
	}))
	defer srv.Close()

	runner := agent.EngineRunner{
		Model: llm.ModelConfig{Name: "fake", BaseURL: srv.URL, APIKey: "x"},
	}
	_, err := runner.Run(t.Context(), job.RunEnv{
		Job: job.Meta{
			Dir:             t.TempDir(),
			Prompt:          "x",
			WorkDir:         outside,
			ParentWorkspace: ws,
		},
		Log: func(string) {},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside parent workspace")
	assert.Zero(t, requests.Load(), "no engine loop may start for an escaping workdir")
}

func TestEngineRunnerCancel(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-block // never stream until cancelled path abandons request
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	defer close(block)

	runner := agent.EngineRunner{
		Model: llm.ModelConfig{Name: "fake", BaseURL: srv.URL, APIKey: "x"},
	}
	mgr, err := job.New(job.Options{
		Root:   t.TempDir(),
		Runner: runner,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })

	info, err := mgr.Spawn(t.Context(), job.SpawnRequest{
		Prompt:  "hang",
		WorkDir: t.TempDir(),
	})
	require.NoError(t, err)

	require.NoError(t, mgr.Cancel(t.Context(), info.ID))
	res, err := mgr.Wait(t.Context(), info.ID)
	require.NoError(t, err)
	assert.Equal(t, job.StatusCancelled, res.Info.Status)
}
