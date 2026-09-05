package agenttool_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/tools/agenttool"
)

func TestAgentWaitPreservesLegacyResultWithoutDeliveryIdentity(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "legacy-job")
	require.NoError(t, os.Mkdir(dir, 0o700))
	resultPath := filepath.Join(dir, "result.md")
	summary := strings.Repeat("x", 13000)
	require.NoError(t, os.WriteFile(resultPath, []byte(summary), 0o600))
	meta := job.Meta{
		ID:         "legacy-job",
		Status:     job.StatusCompleted,
		Role:       job.RoleExplore,
		Dir:        dir,
		ResultPath: resultPath,
	}
	raw, err := json.Marshal(meta)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "meta.json"), raw, 0o600))
	mgr, err := job.New(
		job.Options{
			Root:   root,
			Runner: job.RunnerFunc(func(context.Context, job.RunEnv) (string, error) { return "", nil }),
		},
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })
	waited, err := mgr.Wait(t.Context(), meta.ID)
	require.NoError(t, err)
	require.Equal(t, summary, waited.Summary)
	require.Nil(t, waited.Outcome)
	for _, tool := range agenttool.AgentTools(agenttool.AgentDeps{Manager: mgr}) {
		if tool.Definition.Name != "agent_wait" {
			continue
		}
		for range 2 {
			result, err := tool.Run(t.Context(), mustArgs(t, map[string]any{"job_id": meta.ID}))
			require.NoError(t, err)
			require.Empty(t, result.DeliveryID)
			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(result.Content), &body))
			require.Equal(t, map[string]any{
				"job_id": "legacy-job", "outcome_id": "", "status": "completed", "role": "explore",
				"error": "", "result_path": resultPath, "summary": strings.Repeat("x", 12000) + "\n…(truncated)",
			}, body)
		}
	}
}
