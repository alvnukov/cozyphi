package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestSessionTitleRequiresPermissionWithoutPlan(t *testing.T) {
	for _, approve := range []bool{false, true} {
		dir := t.TempDir()
		gate, err := permission.NewGate(permission.DefaultPolicy(), dir)
		require.NoError(t, err)
		asked := false
		engine, err := NewEngine(EngineOpts{
			Model:       llm.ModelConfig{Name: "fake"},
			SessionOpts: SessionOpts{Cwd: dir, SessionDir: dir},
			Gate:        gate,
			Ask: func(_ context.Context, req permission.Request, _ string) (permission.AskResult, error) {
				require.Equal(t, permission.ActionSession, req.Action)
				asked = true
				return permission.AskResult{Approved: approve}, nil
			},
		})
		require.NoError(t, err)
		_, _, _ = engine.executor.run(
			t.Context(),
			[]llm.ToolCall{
				{
					ID: "title",
					Function: llm.Function{
						Name:      "session",
						Arguments: `{"action":"set_title","title":"Имя основной сессии"}`,
					},
				},
			},
			func(session.ToolData) bool { return true },
		)
		require.True(t, asked, "the plan exemption must not bypass permission")
		title, _ := engine.Session().Title()
		if approve {
			require.Equal(t, "Имя основной сессии", title)
		} else {
			require.Empty(t, title)
		}
		require.NoError(t, engine.Session().Close())
	}
}
