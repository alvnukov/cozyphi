package agenttool_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/tools"
)

func TestAgentToolsSpawnWaitForcesDepthAndParent(t *testing.T) {
	mgr, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(_ context.Context, env job.RunEnv) (string, error) {
			assert.Equal(t, "parent-1", env.Job.ParentID)
			assert.Equal(t, 0, env.Job.ParentDepth)
			return "summary-ok", nil
		}),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(t.Context()) })

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
	spawnRes, err := reg["agent_spawn"].Run(t.Context(), spawnArgs)
	require.NoError(t, err)
	assert.Contains(t, spawnRes.Content, `"role": "explore"`)

	var spawned struct {
		JobID string `json:"job_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(spawnRes.Content), &spawned))

	waitArgs, _ := json.Marshal(map[string]any{"job_id": spawned.JobID})
	waitRes, err := reg["agent_wait"].Run(t.Context(), waitArgs)
	require.NoError(t, err)
	assert.Contains(t, waitRes.Content, "summary-ok")
	assert.Contains(t, waitRes.Content, `"status": "completed"`)
}

func TestAgentToolsSpawnRoleWorker(t *testing.T) {
	mgr, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(_ context.Context, env job.RunEnv) (string, error) {
			assert.Equal(t, job.RoleWorker, env.Job.Role)
			return "done", nil
		}),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(t.Context()) })

	reg := tools.NewRegistry(tools.AgentTools(tools.AgentDeps{
		Manager:  mgr,
		ParentID: func() string { return "p" },
		WorkDir:  func() string { return t.TempDir() },
	}))
	raw, _ := json.Marshal(map[string]any{
		"prompt": "implement x",
		"role":   "worker",
	})
	res, err := reg["agent_spawn"].Run(t.Context(), raw)
	require.NoError(t, err)
	assert.Contains(t, res.Content, `"role": "worker"`)

	var spawned struct {
		JobID string `json:"job_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.Content), &spawned))
	waitArgs, _ := json.Marshal(map[string]any{"job_id": spawned.JobID})
	waitRes, err := reg["agent_wait"].Run(t.Context(), waitArgs)
	require.NoError(t, err)
	assert.Contains(t, waitRes.Content, `"status": "completed"`)
}

func TestAgentToolsSpawnBadRole(t *testing.T) {
	mgr, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(_ context.Context, _ job.RunEnv) (string, error) {
			return "x", nil
		}),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(t.Context()) })

	reg := tools.NewRegistry(tools.AgentTools(tools.AgentDeps{
		Manager: mgr,
	}))
	raw, _ := json.Marshal(map[string]any{"prompt": "x", "role": "nope"})
	_, err = reg["agent_spawn"].Run(t.Context(), raw)
	require.Error(t, err)
}

func TestAgentToolsSpawnResolvesWorkdirAgainstParent(t *testing.T) {
	parent := t.TempDir()
	var gotWorkDir string
	mgr, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(_ context.Context, env job.RunEnv) (string, error) {
			gotWorkDir = env.Job.WorkDir
			return "ok", nil
		}),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(t.Context()) })

	reg := tools.NewRegistry(tools.AgentTools(tools.AgentDeps{
		Manager:  mgr,
		ParentID: func() string { return "p" },
		WorkDir:  func() string { return parent },
	}))

	raw, _ := json.Marshal(map[string]any{"prompt": "x", "workdir": "sub"})
	res, err := reg["agent_spawn"].Run(t.Context(), raw)
	require.NoError(t, err)

	var spawned struct {
		JobID string `json:"job_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.Content), &spawned))
	waitArgs, _ := json.Marshal(map[string]any{"job_id": spawned.JobID})
	_, err = reg["agent_wait"].Run(t.Context(), waitArgs)
	require.NoError(t, err)

	assert.True(t, filepath.IsAbs(gotWorkDir),
		"the child gate treats workdir as its workspace and needs it absolute, got %q", gotWorkDir)
	assert.Equal(t, filepath.Join(parent, "sub"), gotWorkDir)
}

func TestChildToolsExcludeAgent(t *testing.T) {
	for _, tool := range tools.DefaultTools() {
		assert.NotContains(t, tool.Definition.Name, "agent_")
	}
}
