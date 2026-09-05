package agent

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestReplaceSessionBusyPreservesOwner(t *testing.T) {
	dir := t.TempDir()
	makeEngine := func() *Engine {
		e, err := NewEngine(EngineOpts{SessionOpts: SessionOpts{Cwd: dir, SessionDir: dir, Persist: true}})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, e.Session().Close()) })
		require.NoError(t, e.Session().Append(llm.Message{Role: llm.RoleAssistant, Content: "saved"}))
		return e
	}
	first, target := makeEngine(), makeEngine()
	previous := first.Session()
	err := first.ReplaceSession(SessionOpts{ResumePath: target.SessionFile()})
	require.ErrorIs(t, err, session.ErrBusy)
	require.Same(t, previous, first.Session())
	require.NoError(t, previous.Append(llm.Message{Role: llm.RoleAssistant, Content: "still writable"}))
	_, err = session.OpenSession(previous.File())
	require.ErrorIs(t, err, session.ErrBusy)

	require.NoError(t, target.Session().Close())
	require.NoError(t, first.ReplaceSession(SessionOpts{ResumePath: target.SessionFile()}))
	reopened, err := session.OpenSession(previous.File())
	require.NoError(t, err, "successful replacement releases the old owner")
	require.NoError(t, reopened.Close())
}

func TestContinueLastCreatesFreshWhenAllBusy(t *testing.T) {
	dir := t.TempDir()
	opts := SessionOpts{Cwd: dir, SessionDir: dir, Persist: true, ContinueLast: true}
	first, err := NewSession(opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, first.Close()) })
	require.NoError(t, first.Append(llm.Message{Role: llm.RoleAssistant, Content: "busy"}))
	second, err := NewSession(opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, second.Close()) })
	require.NotEqual(t, first.ID(), second.ID())
	require.Empty(t, second.BuildContext())
}

func TestNewSessionRepairFailureReleasesAcquiredOwner(t *testing.T) {
	dir := t.TempDir()
	m, err := session.NewSessionManager(dir, session.WithSessionDir(dir), session.WithShouldFlush(true))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	_, err = m.Append(llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "pending"}}})
	require.NoError(t, err)
	path := m.File()
	require.NoError(t, os.Rename(path, path+".saved"))
	require.NoError(t, os.Mkdir(path, 0o700))
	_, err = NewSession(SessionOpts{Acquired: m})
	require.Error(t, err, "pending-result repair cannot append to a directory")
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Rename(path+".saved", path))
	reopened, err := session.OpenSession(path)
	require.NoError(t, err, "repair failure must not leak ownership")
	require.NoError(t, reopened.Close())
}
