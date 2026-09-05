package agent_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
)

func TestEngineRunnerEffortSelection(t *testing.T) {
	for _, tc := range []struct {
		name                string
		pinned              bool
		effort, model, want string
	}{
		{"role override", true, "high", "role", "high"},
		{"role inheritance", true, "", "role", "low"},
		{"live override", false, "high", "live", "high"},
		{"live inheritance", false, "", "live", "medium"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan map[string]any, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				requests <- body
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", jsonMarshalDelta("done"))
			}))
			defer srv.Close()
			live := llm.ModelConfig{
				Name: "live", BaseURL: srv.URL, APIKey: "x", ReasoningEffort: llm.ReasoningEffortMedium,
				ReasoningEfforts: []llm.ReasoningEffort{llm.ReasoningEffortMedium, llm.ReasoningEffortHigh},
			}
			role := llm.ModelConfig{
				Name: "role", BaseURL: srv.URL, APIKey: "x", ReasoningEffort: llm.ReasoningEffortLow,
				ReasoningEfforts: []llm.ReasoningEffort{llm.ReasoningEffortLow, llm.ReasoningEffortHigh},
			}
			beforeLive, beforeRole := live, role
			runner := agent.EngineRunner{
				Model:        llm.ModelConfig{Name: "stale"},
				ModelFn:      func() llm.ModelConfig { return live },
				ModelForRole: func(r job.Role) (llm.ModelConfig, bool) { assert.Equal(t, job.RoleWorker, r); return role, tc.pinned },
			}
			_, err := runner.Run(t.Context(), job.RunEnv{Job: job.Meta{
				Dir: t.TempDir(), WorkDir: t.TempDir(), Prompt: "probe", Role: job.RoleWorker, Effort: tc.effort,
			}, Log: func(string) {}})
			require.NoError(t, err)
			body := <-requests
			assert.Equal(t, tc.model, body["model"])
			assert.Equal(t, tc.want, body["reasoning_effort"])
			assert.Equal(t, beforeLive, live)
			assert.Equal(t, beforeRole, role)
		})
	}
}

func TestEngineRunnerRejectsEffort(t *testing.T) {
	for _, tc := range []struct {
		name, effort string
		supported    []llm.ReasoningEffort
		message      string
	}{
		{"invalid", "turbo", nil, "invalid job effort"},
		{"no runtime efforts", "high", nil, "unsupported by selected model"},
		{"unsupported role effort", "high", []llm.ReasoningEffort{llm.ReasoningEffortLow}, "unsupported by selected model"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := agent.EngineRunner{
				Model: llm.ModelConfig{
					Name:             "parent",
					ReasoningEfforts: []llm.ReasoningEffort{llm.ReasoningEffortHigh},
				},
				ModelForRole: func(job.Role) (llm.ModelConfig, bool) {
					return llm.ModelConfig{Name: "role", ReasoningEfforts: tc.supported}, true
				},
			}
			_, err := runner.Run(t.Context(), job.RunEnv{Job: job.Meta{
				Dir: t.TempDir(), WorkDir: t.TempDir(), Prompt: "probe", Effort: tc.effort,
			}, Log: func(string) {}})
			require.ErrorContains(t, err, tc.message)
			assert.Contains(t, err.Error(), "re-spawn")
			if tc.effort == "high" {
				assert.Contains(t, err.Error(), `"role"`)
			}
		})
	}
}
