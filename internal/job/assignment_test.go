package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
)

func TestStoppedAssignmentPreservesSessionCorrelation(t *testing.T) {
	manager, err := job.New(
		job.Options{Root: t.TempDir(), Runner: job.RunnerFunc(func(_ context.Context, env job.RunEnv) (string, error) {
			if err := env.BindSession("retained-child-session"); err != nil {
				return "", err
			}
			return "partial answer", &job.StoppedError{Reason: "left_interrupted"}
		})},
	)
	require.NoError(t, err)
	defer manager.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	info, err := manager.Spawn(
		ctx,
		job.SpawnRequest{Prompt: "work", ParentID: "parent-session", ParentToolUseID: "spawn-call"},
	)
	require.NoError(t, err)
	result, err := manager.Wait(ctx, info.ID)
	require.NoError(t, err)
	require.Equal(t, job.StatusCancelled, result.Info.Status)
	require.Equal(t, "left_interrupted", result.Info.StopReason)
	require.Equal(t, "retained-child-session", result.Info.ChildSessionID)
	require.Equal(t, "spawn-call", result.Info.ParentToolUseID)
	require.Equal(t, "partial answer", result.Summary)
}
