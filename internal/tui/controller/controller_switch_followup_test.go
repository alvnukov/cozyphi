package controller

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/job"
)

func TestControllerSwitchReservesTerminalChildFollowUpAdmission(t *testing.T) {
	for _, denied := range []bool{false, true} {
		name := "clear"
		if denied {
			name = "denied"
		}
		t.Run(name, func(t *testing.T) {
			server, bodies := textSSEServer(t)
			parent := newInjectController(t, NewBus(nil), server.URL)
			t.Cleanup(parent.Close)
			parent.runtime.EnableInteractiveChildren()
			parent.Cancel() // Keep parent wake inference out of this child lifecycle test.
			first, err := parent.jobs.SpawnWithRunner(t.Context(), job.SpawnRequest{
				Prompt: "first assignment", Role: job.RoleExplore,
				OwnerID: parent.jobOwnerID, ParentID: parent.engine.SessionID(),
				WorkDir: parent.cwd, ParentWorkspace: parent.cwd,
			}, parent.bindJobRunner(parent.ModelConfig(), parent.Hooks(), nil))
			require.NoError(t, err)
			waitForCond(t, 5*time.Second, func() bool { return len(parent.runtime.Children()) == 1 })
			child := parent.runtime.Children()[0]
			child.Ready(nil)
			old, err := parent.jobs.Wait(t.Context(), first.ID)
			require.NoError(t, err)
			c := child.Controller
			require.True(t, c.Assignment().Terminal)
			previous := c.engine
			c.streamMu.Lock()
			generation := c.streamGen
			c.streamMu.Unlock()

			ready, release := make(chan struct{}), make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			t.Cleanup(unblock)
			manager := hooks.NewManager(hooks.Entry{Kind: hooks.KindSessionBeforeSwitch, Hook: hooks.FuncHook{
				Sess: func(context.Context, hooks.SessionEvent) (hooks.SessionResult, error) {
					c.StartPrompt("queued follow-up marker", nil, "queued-child")
					close(ready)
					<-release
					if denied {
						return hooks.SessionResult{Action: hooks.ActionDeny, Reason: "barrier denied"}, nil
					}
					return hooks.SessionResult{}, nil
				},
			}})
			c.hooksManager.Store(manager)
			done := make(chan error, 1)
			go func() { done <- c.Clear() }()
			awaitSwitchBarrier(t, ready)
			c.streamMu.Lock()
			running, currentGeneration, queued := c.streamRunning, c.streamGen, len(c.promptQueue)
			c.streamMu.Unlock()
			require.False(t, running)
			require.Equal(t, generation, currentGeneration, "switch must reserve writer admission")
			require.Equal(t, 1, queued)
			require.Equal(t, first.ID, c.Assignment().JobID)
			require.Len(t, bodies(), 1)
			unblock()
			select {
			case err = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("switch did not finish")
			}
			if denied {
				require.ErrorContains(t, err, "barrier denied")
				require.Same(t, previous, c.engine)
				require.Same(t, manager, c.Hooks())
			} else {
				require.NoError(t, err)
				require.NotSame(t, previous, c.engine)
			}
			require.NotEqual(t, first.ID, c.Assignment().JobID, "queued input must admit a new assignment")
			next, err := parent.jobs.Wait(t.Context(), c.Assignment().JobID)
			require.NoError(t, err)
			require.Equal(t, first.ID, next.Info.PreviousJobID)
			require.Equal(t, old.Info.OwnerID, next.Info.OwnerID)
			require.Equal(t, old.Info.ParentID, next.Info.ParentID)
			require.Equal(t, c.SessionID(), next.Info.ChildSessionID)
			require.True(t, next.Info.UserIntervened)
			require.True(t, c.Assignment().Terminal)
			require.False(t, c.RunActive())
			requests := bodies()
			require.Len(t, requests, 2)
			require.Contains(t, requests[1], "queued follow-up marker", "promotion alone must not lose the prompt")
			if denied {
				require.Contains(t, requests[1], "first assignment")
			} else {
				require.NotContains(t, requests[1], "first assignment")
			}
			unchanged, err := parent.jobs.Wait(t.Context(), first.ID)
			require.NoError(t, err)
			require.Equal(t, old, unchanged)
		})
	}
}
