package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

func TestSessionTitleOwningStoreAndPin(t *testing.T) {
	dir := t.TempDir()
	engine, err := NewEngine(
		EngineOpts{
			Model:       llm.ModelConfig{Name: "fake"},
			SessionOpts: SessionOpts{Cwd: dir, SessionDir: dir, Persist: true},
		},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Session().Close()) })
	require.True(t, engine.HasTool("session"))
	engine.SetPlanEnabled(false)
	require.True(t, engine.HasTool("session"), "naming is independent of plan mode")
	require.Contains(t, engine.systemPrompt(), "in the user's language")
	require.Contains(t, engine.systemPrompt(), "3–7 words")
	tool := engine.executor.registry["session"]
	result, err := tool.Run(t.Context(), []byte(`{"action":"set_title","title":"Имена текущих сессий"}`))
	require.NoError(t, err)
	require.Equal(t, "Имена текущих сессий", result.Detail)
	title, source := engine.Session().Title()
	require.Equal(t, "Имена текущих сессий", title)
	require.Equal(t, "model", source)
	require.NoError(t, engine.Session().SetTitle("Ручное имя", "user"))
	_, err = tool.Run(t.Context(), []byte(`{"action":"set_title","title":"another name"}`))
	require.ErrorIs(t, err, session.ErrTitlePinned)
	path := engine.SessionFile()
	require.NoError(t, engine.ReplaceSession(SessionOpts{Cwd: dir, SessionDir: dir, Persist: true}))
	_, err = tool.Run(t.Context(), []byte(`{"action":"set_title","title":"must not reach new session"}`))
	require.Error(t, err)
	title, _ = engine.Session().Title()
	require.Empty(t, title)
	reopened, err := session.OpenSession(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, reopened.Close()) }()
	title, source = reopened.Title()
	require.Equal(t, "Ручное имя", title)
	require.Equal(t, "user", source)
}

func TestSessionTitleExcludedFromChildren(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		dir := t.TempDir()
		opts := EngineOpts{
			Model:       llm.ModelConfig{Name: "fake"},
			SessionOpts: SessionOpts{Cwd: dir, SessionDir: dir, ParentID: "parent"},
		}
		if explicit {
			opts.Tools = []tools.Tool{}
		}
		engine, err := NewEngine(opts)
		require.NoError(t, err)
		require.False(t, engine.HasTool("session"))
		require.NotContains(t, engine.systemPrompt(), "# Session naming")
		require.NoError(t, engine.Session().Close())
	}
}
