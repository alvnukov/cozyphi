package controller

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
)

func TestOutcomeInterruptAndConversationFence(t *testing.T) {
	for _, replace := range []bool{false, true} {
		name := "resume-original"
		if replace {
			name = "replace-conversation"
		}
		t.Run(name, func(t *testing.T) {
			server, bodies := textSSEServer(t)
			ctrl := newInjectController(t, NewBus(nil), server.URL)
			defer ctrl.Close()
			ctrl.runtime.EnableInteractiveChildren()
			parentID := ctrl.engine.SessionID()
			ctrl.Cancel()
			info, err := ctrl.jobs.SpawnWithRunner(t.Context(), job.SpawnRequest{
				Prompt: "answer", OwnerID: ctrl.jobOwnerID, ParentID: parentID,
			}, job.RunnerFunc(func(context.Context, job.RunEnv) (string, error) { return "fenced child result", nil }))
			require.NoError(t, err)
			_, err = ctrl.jobs.Wait(t.Context(), info.ID)
			require.NoError(t, err)
			time.Sleep(2 * watchWakeDelay)
			require.Empty(t, bodies(), "an explicit interrupt suppresses later automatic wakes")
			pending, err := ctrl.jobs.PendingOutcomes(t.Context(), ctrl.jobOwnerID, parentID, 1)
			require.NoError(t, err)
			require.Len(t, pending, 1, "interrupt does not discard the result")
			if replace {
				require.NoError(t, ctrl.Clear())
			}
			ctrl.StartPrompt("continue", nil, "resume-input")
			waitForCond(t, 5*time.Second, func() bool { return len(bodies()) > 0 && !ctrl.RunActive() })
			require.Equal(t, !replace, strings.Contains(bodies()[0], "fenced child result"))
			pending, err = ctrl.jobs.PendingOutcomes(t.Context(), ctrl.jobOwnerID, parentID, 1)
			require.NoError(t, err)
			if replace {
				require.Len(t, pending, 1)
			} else {
				require.Empty(t, pending)
			}
		})
	}
}
