package agenttool_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/tools"
)

func TestAgentToolsOwnerScope(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	mgr, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(ctx context.Context, _ job.RunEnv) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		}),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })
	cwd := t.TempDir()
	regs := map[string]tools.Registry{}
	ids := map[string]string{}
	for _, owner := range []string{"a", "b"} {
		reg := tools.NewRegistry(tools.AgentTools(tools.AgentDeps{
			Manager: mgr, OwnerID: owner,
			ParentID: func() string { return "shared-history" },
			WorkDir:  func() string { return cwd },
		}))
		regs[owner] = reg
		assert.NotContains(t, reg["agent_spawn"].Definition.Params.Properties, "owner_id")
		res, err := reg["agent_spawn"].Run(ctx, mustArgs(t, map[string]any{
			"prompt": "run", "skills": []string{}, "no_skill_reason": "probe", "owner_id": "forged",
		}))
		require.NoError(t, err)
		var spawned struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.Unmarshal([]byte(res.Content), &spawned))
		ids[owner] = spawned.JobID
		info, err := mgr.Get(ctx, spawned.JobID)
		require.NoError(t, err)
		assert.Equal(t, owner, info.OwnerID)
		assert.Equal(t, "shared-history", info.ParentID)
	}
	for owner, reg := range regs {
		list, err := reg["agent_list"].Run(ctx, json.RawMessage(`{"owner_id":"forged"}`))
		require.NoError(t, err)
		assert.Contains(t, list.Content, ids[owner])
		for other, id := range ids {
			if other == owner {
				continue
			}
			assert.NotContains(t, list.Content, id)
			for _, name := range []string{"agent_wait", "agent_cancel"} {
				_, err := reg[name].Run(ctx, mustArgs(t, map[string]any{"job_id": id, "owner_id": other}))
				require.ErrorIs(t, err, job.ErrNotFound)
			}
		}
	}
	for owner, reg := range regs {
		args := mustArgs(t, map[string]any{"job_id": ids[owner]})
		_, err := reg["agent_cancel"].Run(ctx, args)
		require.NoError(t, err)
		result, err := reg["agent_wait"].Run(ctx, args)
		require.NoError(t, err)
		assert.Contains(t, result.Content, `"status": "cancelled"`)
	}
	legacy := tools.NewRegistry(tools.AgentTools(tools.AgentDeps{Manager: mgr}))
	list, err := legacy["agent_list"].Run(ctx, json.RawMessage(`{}`))
	require.NoError(t, err)
	for _, id := range ids {
		assert.Contains(t, list.Content, id)
	}
}
