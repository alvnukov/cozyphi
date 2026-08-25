package agent_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/lsp"
	"github.com/alvnukov/cozyphi/internal/tools"
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

func TestLSPRoleMatrices(t *testing.T) {
	readonly, err := agent.NewEngine(agent.EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: agent.SessionOpts{Cwd: t.TempDir(), ParentID: "parent"},
		Tools:       agent.ChildTools(),
		LSP:         fakeLSPQuery,
	})
	require.NoError(t, err)
	assert.True(t, readonly.HasTool("lsp"))
	assert.False(t, readonly.HasTool("write"), "readonly children must not gain write")
	assert.False(t, readonly.HasTool("agent_spawn"), "children never gain agent_* tools")

	worker, err := agent.NewEngine(agent.EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: agent.SessionOpts{Cwd: t.TempDir(), ParentID: "parent"},
		Tools:       tools.DefaultTools(),
		LSP:         fakeLSPQuery,
	})
	require.NoError(t, err)
	assert.True(t, worker.HasTool("lsp"))
	assert.True(t, worker.HasTool("write"), "worker children keep their writable base set")
	assert.False(t, worker.HasTool("agent_spawn"))
}

func TestLSPPreservedAcrossRebinds(t *testing.T) {
	eng := newLSPEngine(t, fakeLSPQuery)
	require.True(t, eng.HasTool("lsp"))

	require.NoError(t, eng.SetModel(llm.ModelConfig{Name: "fake2", BaseURL: "http://127.0.0.1:9", APIKey: "x"}))
	assert.True(t, eng.HasTool("lsp"), "SetModel must preserve the borrowed lsp tool")

	eng.SetMode(agent.ModePlan)
	assert.True(t, eng.HasTool("lsp"), "SetMode must preserve the borrowed lsp tool")
	assert.False(t, eng.HasTool("write"), "plan mode must narrow built-in tools to readonly")

	eng.SetJobs(nil)
	assert.True(t, eng.HasTool("lsp"), "SetJobs must preserve the borrowed lsp tool")
}
