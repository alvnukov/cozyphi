package controller

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestControllerSwitchReservesTurnAdmission(t *testing.T) {
	for _, outcome := range []string{"clear", "resume", "busy", "denied"} {
		for _, userPrompt := range []bool{false, true} {
			name := outcome + "/watch"
			if userPrompt {
				name += "+user"
			}
			t.Run(name, func(t *testing.T) {
				srv, bodies := textSSEServer(t)
				c := newInjectController(t, NewBus(nil), srv.URL)
				t.Cleanup(c.Close)
				previous := c.engine
				require.NoError(t, previous.Session().Append(llm.Message{
					Role: llm.RoleAssistant, Content: "old marker",
				}))

				switchSession := c.Clear
				if outcome == "resume" || outcome == "busy" {
					target, err := c.runtime.NewSession(NewBus(nil), c.workspace, "", nil)
					require.NoError(t, err)
					t.Cleanup(target.Close)
					require.NoError(t, target.engine.Session().Append(llm.Message{
						Role: llm.RoleAssistant, Content: "target marker",
					}))
					id := target.SessionID()
					if outcome == "resume" {
						target.Close()
						<-target.closeDone
					}
					switchSession = func() error { _, err := c.Resume(id); return err }
				}

				ready, release := make(chan struct{}), make(chan struct{})
				unblock := sync.OnceFunc(func() { close(release) })
				t.Cleanup(unblock)
				manager := hooks.NewManager(hooks.Entry{Kind: hooks.KindSessionBeforeSwitch, Hook: hooks.FuncHook{
					Sess: func(context.Context, hooks.SessionEvent) (hooks.SessionResult, error) {
						// Reenter synchronously: holding streamMu around hooks would deadlock.
						c.observeWatchEvent(watchEvent("switch barrier", "watch marker"))
						c.streamMu.Lock()
						if c.watchWake != nil {
							c.watchWake.Stop()
						}
						c.streamMu.Unlock()
						c.wakeForWatches() // force the timer callback; no timing-dependent race window
						if userPrompt {
							c.StartPrompt("user marker", nil, "queued-user")
						}
						c.Compact() // compaction is the other engine-writing admission path
						close(ready)
						<-release
						if outcome == "denied" {
							return hooks.SessionResult{
								Action: hooks.ActionDeny, Reason: "barrier denied",
							}, nil
						}
						return hooks.SessionResult{}, nil
					},
				}})
				c.hooksManager.Store(manager)
				done := make(chan error, 1)
				go func() { done <- switchSession() }()
				awaitSwitchBarrier(t, ready)
				c.streamMu.Lock()
				running, generation, queued := c.streamRunning, c.streamGen, len(c.watchQueue)
				c.streamMu.Unlock()
				require.False(t, running, "no old writer may start inside the before-switch hook")
				require.Zero(t, generation, "admission, not just HTTP delivery, must be blocked")
				require.Equal(t, 1, queued, "the watch must survive commit or rollback")
				require.Empty(t, bodies())
				require.Error(t, c.Clear(), "a second switch must not enter the hook")
				unblock()
				var err error
				select {
				case err = <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("switch did not finish")
				}
				if outcome == "busy" || outcome == "denied" {
					require.Error(t, err)
					if outcome == "busy" {
						require.ErrorIs(t, err, session.ErrBusy)
					}
					require.Same(t, previous, c.engine)
					require.Same(t, manager, c.Hooks())
				} else {
					require.NoError(t, err)
					require.NotSame(t, previous, c.engine)
				}
				waitForCond(t, 5*time.Second, func() bool { return len(bodies()) > 0 && !c.RunActive() })
				require.Len(t, bodies(), 1)
				body := bodies()[0]
				require.Contains(t, body, "watch marker")
				if userPrompt {
					require.Contains(t, body, "user marker")
				}
				if outcome == "busy" || outcome == "denied" {
					require.Contains(t, body, "old marker", "rollback must restore the old engine's admission")
				} else {
					require.NotContains(t, body, "old marker", "only the replacement may run")
					if outcome == "resume" {
						require.Contains(t, body, "target marker")
					}
				}
			})
		}
	}
}

func TestControllerCloseJoinsSessionSwitch(t *testing.T) {
	for _, phase := range []string{"before", "acquired", "committed", "rollback", "reentrant"} {
		t.Run(phase, func(t *testing.T) {
			c := newReadyController(t)
			c.ownsRuntime = false // keep the bounded controller wait isolated from Runtime.Close
			c.closeBudget = 10 * time.Millisecond
			t.Cleanup(func() { require.NoError(t, c.runtime.Close()) })
			t.Cleanup(c.Close)
			previous := c.engine
			require.NoError(t, previous.Session().Append(llm.Message{
				Role: llm.RoleAssistant, Content: "old owner",
			}))
			ready, release := make(chan struct{}), make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			t.Cleanup(unblock)
			kind := hooks.KindSessionBeforeSwitch
			switch phase {
			case "acquired":
				kind = hooks.KindSessionShutdown
			case "committed":
				kind = hooks.KindSessionStart
			}
			startupWrite := make(chan error, 1)
			c.hooksManager.Store(hooks.NewManager(hooks.Entry{Kind: hooks.KindSessionStart, Hook: hooks.FuncHook{
				Sess: func(context.Context, hooks.SessionEvent) (hooks.SessionResult, error) {
					// Fresh sessions flush lazily. Persist the replacement so reopening
					// below proves release of a real owner rather than a missing file.
					startupWrite <- c.engine.Session().Append(llm.Message{
						Role: llm.RoleAssistant, Content: "replacement owner",
					})
					return hooks.SessionResult{}, nil
				},
			}}, hooks.Entry{Kind: kind, Hook: hooks.FuncHook{
				Sess: func(_ context.Context, ev hooks.SessionEvent) (hooks.SessionResult, error) {
					if ev.Reason == "quit" {
						return hooks.SessionResult{}, nil
					}
					if phase == "reentrant" {
						c.Close()
					}
					close(ready)
					<-release
					if phase == "rollback" {
						return hooks.SessionResult{Action: hooks.ActionDeny}, nil
					}
					return hooks.SessionResult{}, nil
				},
			}}))
			done := make(chan error, 1)
			go func() { done <- c.Clear() }()
			awaitSwitchBarrier(t, ready)
			c.Close()
			select {
			case <-c.closeDone:
				t.Fatal("cleanup released an owner while the switch hook was still running")
			default:
			}
			// The hook barrier makes the current engine stable. A committed
			// replacement must remain writable until the switch finishes too.
			if phase == "committed" {
				// Session-start notification hooks run in parallel. Join the test
				// writer before appending through the single-writer agent session.
				require.NoError(t, <-startupWrite)
			}
			current := c.engine
			require.NoError(t, current.Session().Append(llm.Message{
				Role: llm.RoleAssistant, Content: "hook still owns writes",
			}))
			_, err := session.OpenSession(current.SessionFile())
			require.ErrorIs(t, err, session.ErrBusy)
			c.StartPrompt("must not start after Close", nil, "")
			c.wakeForWatches()
			c.streamMu.Lock()
			generation := c.streamGen
			c.streamMu.Unlock()
			require.Zero(t, generation)
			unblock()
			select {
			case err = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("switch did not finish after Close")
			}
			if phase == "rollback" {
				require.Error(t, err)
				require.Same(t, previous, c.engine)
			} else {
				require.NoError(t, err)
				if phase != "committed" {
					require.NoError(t, <-startupWrite, "Close must retain the replacement through startup writes")
				}
				require.NotSame(t, previous, c.engine)
			}
			awaitSwitchBarrier(t, c.closeDone)
			for _, path := range []string{previous.SessionFile(), c.SessionFile()} {
				owner, openErr := session.OpenSession(path)
				require.NoError(t, openErr, "neither the old owner nor replacement may leak")
				require.NoError(t, owner.Close())
			}
		})
	}
}

func TestControllerSwitchCancelKeepsWatchQueued(t *testing.T) {
	srv, bodies := textSSEServer(t)
	c := newInjectController(t, NewBus(nil), srv.URL)
	t.Cleanup(c.Close)
	c.hooksManager.Store(hooks.NewManager(hooks.Entry{Kind: hooks.KindSessionBeforeSwitch, Hook: hooks.FuncHook{
		Sess: func(context.Context, hooks.SessionEvent) (hooks.SessionResult, error) {
			c.observeWatchEvent(watchEvent("cancelled wake", "watch marker"))
			c.wakeForWatches()
			c.Cancel()
			return hooks.SessionResult{Action: hooks.ActionDeny}, nil
		},
	}}))
	require.Error(t, c.Clear())
	c.streamMu.Lock()
	generation, queued, armed := c.streamGen, len(c.watchQueue), c.watchWake != nil
	c.streamMu.Unlock()
	require.Zero(t, generation)
	require.Equal(t, 1, queued)
	require.False(t, armed)
	c.StartPrompt("resume after cancel", nil, "")
	waitForCond(t, 5*time.Second, func() bool { return len(bodies()) > 0 && !c.RunActive() })
	require.Len(t, bodies(), 1)
	require.Contains(t, bodies()[0], "watch marker")
}

func awaitSwitchBarrier(t *testing.T, ready <-chan struct{}) {
	t.Helper()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("switch hook deadlocked")
	}
}
