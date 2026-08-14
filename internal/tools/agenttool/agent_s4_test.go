package agenttool_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/agent"
	"github.com/pulseaiclub/phi/internal/job"
	"github.com/pulseaiclub/phi/internal/tools"
)

// S4 acceptance: dual spawn+wait, cancel, no nested agent tools, parent sees
// only short summaries (not child transcripts).

func TestS4DualSpawnWait(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]bool{}
	mgr, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(_ context.Context, env job.RunEnv) (string, error) {
			mu.Lock()
			seen[env.Job.Description] = true
			mu.Unlock()
			env.Log(strings.Repeat("child-trace-line\n", 200))
			return "summary:" + env.Job.Description, nil
		}),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(t.Context()) })

	reg := tools.NewRegistry(tools.AgentTools(tools.AgentDeps{
		Manager:  mgr,
		ParentID: func() string { return "parent-s4" },
		WorkDir:  func() string { return t.TempDir() },
	}))

	spawn := func(desc string) string {
		raw, _ := json.Marshal(map[string]any{"prompt": "p-" + desc, "description": desc})
		res, err := reg["agent_spawn"].Run(t.Context(), raw)
		require.NoError(t, err)
		var out struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.Unmarshal([]byte(res.Content), &out))
		return out.JobID
	}

	idA := spawn("A")
	idB := spawn("B")
	require.NotEqual(t, idA, idB)

	wait := func(id string) string {
		raw, _ := json.Marshal(map[string]any{"job_id": id})
		res, err := reg["agent_wait"].Run(t.Context(), raw)
		require.NoError(t, err)
		var out struct {
			Status  string `json:"status"`
			Summary string `json:"summary"`
		}
		require.NoError(t, json.Unmarshal([]byte(res.Content), &out))
		assert.Equal(t, "completed", out.Status)
		return out.Summary
	}

	sumA := wait(idA)
	sumB := wait(idB)
	assert.Equal(t, "summary:A", sumA)
	assert.Equal(t, "summary:B", sumB)
	assert.True(t, seen["A"] && seen["B"])

	// Parent tool payload stays small despite huge child logs on disk.
	assert.Less(t, len(sumA)+len(sumB), 200)
	events, err := mgr.Log(t.Context(), idA, 0)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	var sb strings.Builder
	for _, ev := range events {
		_, _ = sb.WriteString(ev.Message)
	}
	joined := sb.String()
	assert.Contains(t, joined, "child-trace-line")
	assert.Greater(t, len(joined), 1000) // bulky transcript stayed in job log, not parent summary
}

func TestS4Cancel(t *testing.T) {
	started := make(chan struct{})
	mgr, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(ctx context.Context, _ job.RunEnv) (string, error) {
			close(started)
			<-ctx.Done()
			return "", ctx.Err()
		}),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(t.Context()) })

	reg := tools.NewRegistry(tools.AgentTools(tools.AgentDeps{
		Manager:  mgr,
		ParentID: func() string { return "p" },
		WorkDir:  func() string { return t.TempDir() },
	}))

	raw, _ := json.Marshal(map[string]any{"prompt": "hang"})
	spawnRes, err := reg["agent_spawn"].Run(t.Context(), raw)
	require.NoError(t, err)
	var spawned struct {
		JobID string `json:"job_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(spawnRes.Content), &spawned))

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not start")
	}

	cancelRaw, _ := json.Marshal(map[string]any{"job_id": spawned.JobID})
	_, err = reg["agent_cancel"].Run(t.Context(), cancelRaw)
	require.NoError(t, err)

	waitRaw, _ := json.Marshal(map[string]any{"job_id": spawned.JobID})
	waitRes, err := reg["agent_wait"].Run(t.Context(), waitRaw)
	require.NoError(t, err)
	assert.Contains(t, waitRes.Content, `"status": "cancelled"`)
}

func TestS4ChildToolsNoAgentSpawn(t *testing.T) {
	for _, tool := range agent.ChildTools() {
		assert.NotEqual(t, "agent_spawn", tool.Definition.Name)
		assert.False(t, strings.HasPrefix(tool.Definition.Name, "agent_"))
		assert.NotEqual(t, "write", tool.Definition.Name)
		assert.NotEqual(t, "edit", tool.Definition.Name)
	}
}

func TestS4AgentTask(t *testing.T) {
	mgr, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(_ context.Context, env job.RunEnv) (string, error) {
			require.Equal(t, 0, env.Job.ParentDepth)
			require.Equal(t, "parent-task", env.Job.ParentID)
			return "task-done", nil
		}),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(t.Context()) })

	reg := tools.NewRegistry(tools.AgentTools(tools.AgentDeps{
		Manager:  mgr,
		ParentID: func() string { return "parent-task" },
		WorkDir:  func() string { return t.TempDir() },
	}))
	require.Contains(t, reg, "agent_task")

	raw, _ := json.Marshal(map[string]any{"prompt": "one shot", "description": "t"})
	res, err := reg["agent_task"].Run(t.Context(), raw)
	require.NoError(t, err)
	assert.Contains(t, res.Content, "task-done")

	// result.md exists
	var out struct {
		ResultPath string `json:"result_path"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.Content), &out))
	data, err := os.ReadFile(out.ResultPath)
	require.NoError(t, err)
	assert.Equal(t, "task-done", string(data))
	_, err = os.Stat(filepath.Dir(out.ResultPath))
	require.NoError(t, err)
}
