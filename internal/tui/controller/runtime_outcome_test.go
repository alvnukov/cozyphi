package controller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
)

func TestRuntimeCloseReportsUndeliveredOutcome(t *testing.T) {
	server, _ := textSSEServer(t)
	parent := newInjectController(t, NewBus(nil), server.URL)
	defer parent.Close()
	parent.Cancel()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	info, err := parent.jobs.SpawnWithRunner(ctx, job.SpawnRequest{
		Prompt: "work", OwnerID: parent.jobOwnerID, ParentID: parent.engine.SessionID(),
	}, job.RunnerFunc(func(_ context.Context, env job.RunEnv) (string, error) {
		if err := os.Mkdir(filepath.Join(env.Job.Dir, "meta.json.tmp"), 0o700); err != nil {
			return "", err
		}
		return "must not disappear", nil
	}))
	require.NoError(t, err)
	defer func() { require.NoError(t, os.Remove(filepath.Join(info.Dir, "meta.json.tmp"))) }()
	_, err = parent.jobs.Wait(ctx, info.ID)
	require.ErrorContains(t, err, "undelivered outcome")
	require.ErrorContains(
		t,
		parent.runtime.Close(),
		"undelivered outcome",
		"the CLI must be able to report shutdown data loss",
	)
}
