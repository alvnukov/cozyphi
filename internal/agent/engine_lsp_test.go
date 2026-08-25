package agent_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/lsp"
)

func fakeLSPQuery(context.Context, lsp.Query) (lsp.Result, error) {
	return lsp.Result{}, nil
}

func newLSPEngine(t *testing.T, q lsp.QueryFunc) *agent.Engine {
	t.Helper()
	eng, err := agent.NewEngine(agent.EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: agent.SessionOpts{Cwd: t.TempDir()},
		LSP:         q,
	})
	require.NoError(t, err)
	return eng
}

func TestLSPToolRegistration(t *testing.T) {
	eng := newLSPEngine(t, fakeLSPQuery)
	assert.True(t, eng.HasTool("lsp"))

	plain, err := agent.NewEngine(agent.EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: agent.SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)
	assert.False(t, plain.HasTool("lsp"), "a nil query must disable the lsp tool entirely")
}

func TestLSPToolSurvivesRebind(t *testing.T) {
	eng := newLSPEngine(t, fakeLSPQuery)
	require.True(t, eng.HasTool("lsp"))
	eng.SetJobs(nil)
	assert.True(t, eng.HasTool("lsp"), "rebinding must not drop the borrowed lsp tool")
}
