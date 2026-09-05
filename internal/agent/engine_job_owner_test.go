package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tools"
)

func TestLoopJobOwnerSurvivesSessionReplacement(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	server := boundSpawnServer(t, job.RoleExplore, nil)
	manager, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(context.Context, job.RunEnv) (string, error) {
			return "done", nil
		}),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Close() })
	opts := EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Tools:       []tools.Tool{},
		Gate:        permission.AllowAll{},
		Jobs:        manager,
		JobOwnerID:  "controller-lifetime",
	}
	engine, err := NewEngine(opts)
	require.NoError(t, err)
	engine.SetMode(ModeBuild)
	parents := make([]string, 0, 2)
	for i := range 2 {
		parents = append(parents, engine.SessionID())
		spawn, err := boundSpawnLoop(ctx, engine)
		require.NoError(t, err)
		result, err := manager.Wait(ctx, spawn.JobID)
		require.NoError(t, err)
		assert.Equal(t, "controller-lifetime", result.Info.OwnerID)
		assert.Equal(t, parents[i], result.Info.ParentID)
		if i == 0 {
			require.NoError(t, engine.ReplaceSession(SessionOpts{Cwd: t.TempDir()}))
			engine.RefreshTools()
		}
	}
	assert.NotEqual(t, parents[0], parents[1])
}
