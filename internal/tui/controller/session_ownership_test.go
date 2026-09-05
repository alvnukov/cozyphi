package controller

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestControllerBusyResumePreservesSessionAndHooks(t *testing.T) {
	c := newReadyController(t)
	t.Cleanup(c.Close)
	target, err := c.runtime.NewSession(NewBus(nil), c.workspace, "", nil)
	require.NoError(t, err)
	t.Cleanup(target.Close)
	for _, owner := range []*Controller{c, target} {
		require.NoError(t, owner.engine.Session().Append(llm.Message{Role: llm.RoleAssistant, Content: "saved"}))
	}
	var shutdowns atomic.Int32
	manager := hooks.NewManager(hooks.Entry{Kind: hooks.KindSessionShutdown, Hook: hooks.FuncHook{
		Sess: func(context.Context, hooks.SessionEvent) (hooks.SessionResult, error) {
			shutdowns.Add(1)
			return hooks.SessionResult{}, nil
		},
	}})
	c.hooksManager.Store(manager)
	previous := c.engine
	_, err = c.Resume(target.SessionID())
	require.ErrorIs(t, err, session.ErrBusy)
	require.Same(t, previous, c.engine)
	require.Same(t, manager, c.Hooks())
	require.Zero(t, shutdowns.Load(), "a failed acquisition must not shut down the prior session")
	require.NoError(t, c.engine.Session().Append(llm.Message{Role: llm.RoleAssistant, Content: "still here"}))
	_, err = session.OpenSession(previous.SessionFile())
	require.ErrorIs(t, err, session.ErrBusy)

	target.Close()
	<-target.closeDone
	_, err = c.Resume(target.SessionID())
	require.NoError(t, err)
	require.EqualValues(t, 1, shutdowns.Load())
	reopened, err := session.OpenSession(previous.SessionFile())
	require.NoError(t, err)
	require.NoError(t, reopened.Close())
	c.Close()
	<-c.closeDone
	reopened, err = session.OpenSession(c.SessionFile())
	require.NoError(t, err)
	require.NoError(t, reopened.Close())
}

func TestControllerCloseRetainsOwnerUntilWorkersExit(t *testing.T) {
	c := newReadyController(t)
	c.ownsRuntime = false // exercise only this controller's bounded wait
	t.Cleanup(c.runtime.Close)
	c.closeBudget = 10 * time.Millisecond
	require.NoError(t, c.engine.Session().Append(llm.Message{Role: llm.RoleAssistant, Content: "saved"}))
	ready, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	_, err := c.jobs.SpawnWithRunner(t.Context(), job.SpawnRequest{
		Prompt: "hold ownership", WorkDir: c.cwd, OwnerID: c.jobOwnerID, ParentID: c.SessionID(),
	}, job.RunnerFunc(func(context.Context, job.RunEnv) (string, error) {
		close(ready)
		<-release
		return "done", nil
	}))
	require.NoError(t, err)
	<-ready
	c.Close()
	_, err = session.OpenSession(c.SessionFile())
	require.ErrorIs(t, err, session.ErrBusy, "bounded Close is not proof of worker completion")
	unblock()
	select {
	case <-c.closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("actual cleanup did not finish")
	}
	reopened, err := session.OpenSession(c.SessionFile())
	require.NoError(t, err)
	require.NoError(t, reopened.Close())
}

func TestRuntimeAdoptsAcquiredSessionAndCleansFailedAdmission(t *testing.T) {
	c := newReadyController(t)
	t.Cleanup(c.Close)
	m, err := session.NewSessionManager(c.cwd, session.WithSessionDir(c.sessionDir), session.WithShouldFlush(true))
	require.NoError(t, err)
	_, err = m.Append(llm.Message{Role: llm.RoleAssistant, Content: "saved"})
	require.NoError(t, err)
	path := m.File()
	require.NoError(t, m.Close())
	m, err = session.OpenLatestSession(c.sessionDir)
	require.NoError(t, err)
	adopted, err := c.runtime.NewSession(NewBus(nil), c.workspace, path, m)
	require.NoError(t, err, "startup must not reopen its own acquired session")
	t.Cleanup(adopted.Close)
	require.Equal(t, m.ID(), adopted.SessionID())
	adopted.Close()
	<-adopted.closeDone
	m, err = session.OpenSession(path)
	require.NoError(t, err)
	_, err = c.runtime.NewSession(nil, c.workspace, path, m)
	require.Error(t, err)
	reopened, err := session.OpenSession(path)
	require.NoError(t, err, "failed admission consumes and releases the handed-off owner")
	require.NoError(t, reopened.Close())
}
