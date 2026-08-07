package agenttool_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pulseaiclub/phi/internal/job"
	"github.com/pulseaiclub/phi/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentToolsSpawnWaitForcesDepthAndParent(t *testing.T) {
	mgr, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(ctx context.Context, env job.RunEnv) (string, error) {
			assert.Equal(t, "parent-1", env.Job.ParentID)
			assert.Equal(t, 0, env.Job.ParentDepth)
			return "summary-ok", nil
		}),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(context.Background()) })

	reg := tools.NewRegistry(tools.AgentTools(tools.AgentDeps{
		Manager:  mgr,
		ParentID: func() string { return "parent-1" },
		WorkDir:  func() string { return t.TempDir() },
	}))
	require.Contains(t, reg, "agent_spawn")
	require.Contains(t, reg, "agent_wait")
	require.NotContains(t, tools.NewRegistry(tools.DefaultTools()), "agent_spawn")

	spawnArgs, _ := json.Marshal(map[string]any{
		"prompt":      "do work",
		"description": "d",
		"depth":       99, // must be ignored
		"parent_id":   "evil",
	})
	spawnRes, err := reg["agent_spawn"].Run(context.Background(), spawnArgs)
	require.NoError(t, err)

	var spawned struct {
		JobID string `json:"job_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(spawnRes.Content), &spawned))

	waitArgs, _ := json.Marshal(map[string]any{"job_id": spawned.JobID})
	waitRes, err := reg["agent_wait"].Run(context.Background(), waitArgs)
	require.NoError(t, err)
	assert.Contains(t, waitRes.Content, "summary-ok")
	assert.Contains(t, waitRes.Content, `"status": "completed"`)
}

func TestChildToolsExcludeAgent(t *testing.T) {
	for _, tool := range tools.DefaultTools() {
		assert.NotContains(t, tool.Definition.Name, "agent_")
	}
}
