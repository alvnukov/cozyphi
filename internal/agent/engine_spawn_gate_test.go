package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/job"
	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/permission"
	"github.com/pulseaiclub/phi/internal/session"
)

// TestLoopAgentSpawnWorkdirEscapeFailsSync pins the sub-agent boundary: a
// spawn whose workdir falls outside the session workspace is rejected by
// spawn validation — a synchronous tool error the model can correct — and
// never creates a job. The child would treat its workdir as the write
// boundary, so an unchecked workdir widens the parent's workspace silently.
func TestLoopAgentSpawnWorkdirEscapeFailsSync(t *testing.T) {
	cwd := t.TempDir()
	outside := t.TempDir() // sibling of cwd, outside the workspace

	var spawned atomic.Int32
	mgr, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(_ context.Context, _ job.RunEnv) (string, error) {
			spawned.Add(1)
			return "ran", nil
		}),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(t.Context()) })

	gate, err := permission.NewGate(permission.DefaultPolicy(), cwd)
	require.NoError(t, err)

	spawnArgs, err := json.Marshal(map[string]any{
		"prompt":  "touch files",
		"role":    "worker",
		"workdir": outside,
	})
	require.NoError(t, err)

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if requests.Add(1) == 1 {
			_, _ = fmt.Fprint(w, sseToolCallChunk("call_1", "agent_spawn", string(spawnArgs)))
		} else {
			_, _ = fmt.Fprint(w, sseTextChunk())
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	var asks atomic.Int32
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: cwd},
		Gate:        gate,
		Ask: func(_ context.Context, _ permission.Request, _ string) (permission.AskResult, error) {
			asks.Add(1)
			return permission.AskResult{Approved: false}, nil
		},
	})
	require.NoError(t, err)
	engine.SetJobs(mgr)
	// Isolate spawn-validation behavior from plan gating: useplan is the
	// default posture and would deny the unplanned call before validation runs.
	engine.SetMode(ModeBuild)

	spawnStatus := ""
	var lastErr error
	for ev, loopErr := range engine.Loop(t.Context(), "spawn a worker", LoopOpts{}) {
		if loopErr != nil {
			lastErr = loopErr
			break
		}
		if td, ok := ev.(session.ToolData); ok && td.Run.Name == "agent_spawn" {
			spawnStatus = td.Run.Status.String()
		}
	}
	require.NoError(t, lastErr)

	require.Equal(t, int32(0), asks.Load(), "confinement is spawn validation, not a user prompt")
	require.Equal(t, int32(0), spawned.Load(), "no job may be created for an escaping workdir")
	require.Equal(t, session.ToolError.String(), spawnStatus)
	require.Equal(t, int32(2), requests.Load(), "model must see the error and answer")
}
