package job_test

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
)

func TestTerminalOutcomePersistsUntilCorrelatedAcknowledgement(t *testing.T) {
	root := t.TempDir()
	runner := job.RunnerFunc(func(_ context.Context, env job.RunEnv) (string, error) {
		if err := env.BindSession("child-session"); err != nil {
			return "", err
		}
		return strings.Repeat("界", 5000), &job.StoppedError{Reason: "left_interrupted"}
	})
	manager, err := job.New(job.Options{Root: root, Runner: runner})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	info, err := manager.Spawn(
		t.Context(),
		job.SpawnRequest{
			Prompt:          "work",
			OwnerID:         "live-parent",
			ParentID:        "parent-conversation",
			ParentToolUseID: "spawn-call",
		},
	)
	require.NoError(t, err)
	result, err := manager.Wait(t.Context(), info.ID)
	require.NoError(t, err)
	outcomes, err := manager.PendingOutcomes(t.Context(), "live-parent", "parent-conversation", 1)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	outcome := outcomes[0]
	require.Equal(t, result.Info.OutcomeID, outcome.EventID)
	require.NotEmpty(t, outcome.EventID)
	require.Equal(t, info.ID, outcome.JobID)
	require.Equal(t, "child-session", outcome.ChildSessionID)
	require.Equal(t, "spawn-call", outcome.ParentToolUseID)
	require.Equal(t, job.StatusCancelled, outcome.Status)
	require.Equal(t, "left_interrupted", outcome.StopReason)
	require.LessOrEqual(t, len(outcome.Summary), 12000)
	require.True(t, utf8.ValidString(outcome.Summary))
	require.NotEmpty(t, outcome.Summary)
	wrong, err := manager.PendingOutcomes(t.Context(), "live-parent", "replacement-conversation", 10)
	require.NoError(t, err)
	require.Empty(t, wrong)
	require.Error(
		t,
		manager.AcknowledgeOutcome(t.Context(), "other-owner", "parent-conversation", info.ID, outcome.EventID),
	)
	require.Error(
		t,
		manager.AcknowledgeOutcome(t.Context(), "live-parent", "parent-conversation", info.ID, "stale-event"),
	)
	manager.Close()
	reopened, err := job.New(job.Options{Root: root, Runner: runner})
	require.NoError(t, err)
	defer reopened.Close()
	pending, err := reopened.PendingOutcomes(t.Context(), "live-parent", "parent-conversation", 1)
	require.NoError(t, err)
	require.Equal(t, outcomes, pending)
	require.NoError(
		t,
		reopened.AcknowledgeOutcome(t.Context(), "live-parent", "parent-conversation", info.ID, outcome.EventID),
	)
	require.NoError(
		t,
		reopened.AcknowledgeOutcome(t.Context(), "live-parent", "parent-conversation", info.ID, outcome.EventID),
	)
	pending, err = reopened.PendingOutcomes(t.Context(), "live-parent", "parent-conversation", 1)
	require.NoError(t, err)
	require.Empty(t, pending)
	waited, err := reopened.Wait(t.Context(), info.ID)
	require.NoError(t, err)
	require.Equal(t, result, waited)
}
