package job_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
)

func TestSpawnPersistsEffort(t *testing.T) {
	for _, effort := range []string{"", "high"} {
		t.Run(effort, func(t *testing.T) {
			received := make(chan string, 1)
			mgr := newMgr(t, job.RunnerFunc(func(_ context.Context, env job.RunEnv) (string, error) {
				received <- env.Job.Effort
				return "done", nil
			}), job.Options{})
			info, err := mgr.Spawn(t.Context(), job.SpawnRequest{Prompt: "probe", Effort: effort})
			require.NoError(t, err)
			assert.Equal(t, effort, info.Effort)
			result, err := mgr.Wait(t.Context(), info.ID)
			require.NoError(t, err)
			assert.Equal(t, effort, result.Info.Effort)
			assert.Equal(t, effort, <-received)
			raw, err := os.ReadFile(filepath.Join(info.Dir, "meta.json"))
			require.NoError(t, err)
			var meta job.Meta
			require.NoError(t, json.Unmarshal(raw, &meta))
			assert.Equal(t, effort, meta.Effort)
			if effort == "" {
				assert.NotContains(t, string(raw), `"effort"`)
			}
		})
	}
}
