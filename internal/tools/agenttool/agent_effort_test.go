package agenttool_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/tools"
)

func TestAgentSpawnEffort(t *testing.T) {
	for _, effort := range []string{"", "none", "minimal", "low", "medium", "high", "xhigh", "max", " HIGH "} {
		t.Run(effort, func(t *testing.T) {
			received := make(chan job.Meta, 1)
			mgr, err := job.New(
				job.Options{
					Root: t.TempDir(),
					Runner: job.RunnerFunc(func(_ context.Context, env job.RunEnv) (string, error) {
						received <- env.Job
						return "done", nil
					}),
				},
			)
			require.NoError(t, err)
			t.Cleanup(func() { _ = mgr.Close() })
			reg := tools.NewRegistry(tools.AgentTools(tools.AgentDeps{Manager: mgr}))
			schema := reg["agent_spawn"].Definition.Params
			assert.Contains(t, schema.Properties, "effort")
			assert.NotContains(t, schema.Properties, "model")
			assert.NotContains(t, schema.Required, "effort")
			res, err := reg["agent_spawn"].Run(t.Context(), mustArgs(t, map[string]any{
				"prompt": "probe", "skills": []string{}, "no_skill_reason": "none fits", "effort": effort,
				"unknown": true, "depth": 99, "parent_id": "ignored",
			}))
			require.NoError(t, err)
			var spawned struct {
				JobID string `json:"job_id"`
			}
			require.NoError(t, json.Unmarshal([]byte(res.Content), &spawned))
			result, err := mgr.Wait(t.Context(), spawned.JobID)
			require.NoError(t, err)
			want := effort
			if effort == " HIGH " {
				want = "high"
			}
			assert.Equal(t, want, result.Info.Effort)
			assert.Equal(t, want, (<-received).Effort)
			raw, err := os.ReadFile(filepath.Join(result.Info.Dir, "meta.json"))
			require.NoError(t, err)
			var meta job.Meta
			require.NoError(t, json.Unmarshal(raw, &meta))
			assert.Equal(t, want, meta.Effort)
		})
	}
}

func TestAgentSpawnRejectsEffortAndModelInput(t *testing.T) {
	mgr, err := job.New(
		job.Options{Root: t.TempDir(), Runner: job.RunnerFunc(func(_ context.Context, _ job.RunEnv) (string, error) {
			t.Error("invalid input must not launch a job")
			return "", nil
		})},
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })
	reg := tools.NewRegistry(tools.AgentTools(tools.AgentDeps{Manager: mgr}))
	for _, tc := range []struct {
		name, field string
		value       any
		message     string
	}{
		{"invalid effort", "effort", "turbo", "invalid effort"},
		{"nonstring effort", "effort", 42, "cannot unmarshal"},
		{"model", "model", "other", "model selection is user-controlled"},
		{"empty model", "model", "", "model selection is user-controlled"},
		{"null model", "model", nil, "model selection is user-controlled"},
		{"object model", "model", map[string]any{}, "model selection is user-controlled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{
				"prompt":          "probe",
				"skills":          []string{},
				"no_skill_reason": "none fits",
				tc.field:          tc.value,
			}
			_, err := reg["agent_spawn"].Run(t.Context(), mustArgs(t, args))
			require.ErrorContains(t, err, tc.message)
		})
	}
}
