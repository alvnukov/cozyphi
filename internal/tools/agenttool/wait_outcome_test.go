package agenttool_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/tools/agenttool"
)

func TestAgentWaitDeliversCompletePersistedOutcome(t *testing.T) {
	for _, tc := range []struct {
		name   string
		runErr error
		status job.Status
		reason string
	}{
		{"failed", errors.New("provider failed"), job.StatusFailed, ""},
		{"stopped", &job.StoppedError{Reason: "left_interrupted"}, job.StatusCancelled, "left_interrupted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			summary := strings.Repeat("界", 5000)
			runner := job.RunnerFunc(func(_ context.Context, env job.RunEnv) (string, error) {
				if err := env.BindSession("child-session"); err != nil {
					return "", err
				}
				env.MarkIntervention()
				return summary, tc.runErr
			})
			mgr, err := job.New(job.Options{Root: root, Runner: runner})
			require.NoError(t, err)
			t.Cleanup(func() { _ = mgr.Close() })
			info, err := mgr.Spawn(
				t.Context(),
				job.SpawnRequest{
					Prompt:          "work",
					OwnerID:         "owner",
					ParentID:        "parent",
					ParentToolUseID: "spawn-call",
					PreviousJobID:   "previous",
				},
			)
			require.NoError(t, err)
			waited, err := mgr.Wait(t.Context(), info.ID)
			require.NoError(t, err)
			require.NotEmpty(t, waited.Summary)
			pending, err := mgr.PendingOutcomes(t.Context(), "owner", "parent", 1)
			require.NoError(t, err)
			require.Len(t, pending, 1)
			outcome := pending[0]
			require.Equal(t, strings.Repeat("界", 4000), outcome.Summary)
			require.Equal(t, tc.status, outcome.Status)
			require.Equal(t, tc.reason, outcome.StopReason)
			require.True(t, outcome.UserIntervened)
			require.Equal(t, "child-session", outcome.ChildSessionID)
			require.Equal(t, "spawn-call", outcome.ParentToolUseID)
			require.Equal(t, "previous", outcome.PreviousJobID)
			require.Equal(t, "owner", outcome.OwnerID)
			require.Equal(t, "parent", outcome.ParentSessionID)
			require.Equal(t, tc.runErr.Error(), outcome.Error)
			require.NoError(t, mgr.Close())
			reopened, err := job.New(job.Options{Root: root, Runner: runner})
			require.NoError(t, err)
			t.Cleanup(func() { _ = reopened.Close() })
			for _, tool := range agenttool.AgentTools(agenttool.AgentDeps{Manager: reopened, OwnerID: "owner"}) {
				if tool.Definition.Name != "agent_wait" {
					continue
				}
				for range 2 {
					result, err := tool.Run(t.Context(), mustArgs(t, map[string]any{"job_id": info.ID}))
					require.NoError(t, err)
					var got job.Outcome
					require.NoError(t, json.Unmarshal([]byte(result.Content), &got))
					require.Equal(t, outcome, got)
					require.Equal(t, outcome.EventID, result.DeliveryID)
					require.Equal(t, result.Content, result.Output)
					require.NoError(
						t,
						reopened.AcknowledgeOutcome(t.Context(), "owner", "parent", info.ID, result.DeliveryID),
					)
				}
			}
		})
	}
}
