package agent_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/llm"
)

func newEngineWithDiagnostics(t *testing.T, registry *diag.Registry) *agent.Engine {
	t.Helper()
	eng, err := agent.NewEngine(agent.EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: agent.SessionOpts{Cwd: t.TempDir()},
		Diagnostics: registry,
	})
	require.NoError(t, err)
	return eng
}

func developerRegistry() *diag.Registry {
	return diag.NewRegistry(nil, diag.DefaultLimits(), diag.NewRuntimeCollector(diag.RuntimeDeps{
		Version: "1.4.2",
		Mode:    "headless",
		Enabled: true,
	}))
}

func TestHarnessIsRegisteredOnlyWithTheDeveloperCapability(t *testing.T) {
	assert.False(t, newEngineWithDiagnostics(t, nil).HasTool("harness"),
		"without --developer-mode the tool must not exist at all")
	assert.True(t, newEngineWithDiagnostics(t, developerRegistry()).HasTool("harness"))
}

func TestHarnessSurvivesToolListRebuilds(t *testing.T) {
	eng := newEngineWithDiagnostics(t, developerRegistry())

	require.NoError(t, eng.SetModel(llm.ModelConfig{Name: "fake2", BaseURL: "http://127.0.0.1:9", APIKey: "x"}))
	assert.True(t, eng.HasTool("harness"), "the capability is the process's, not the model's")

	eng.SetJobs(nil)
	assert.True(t, eng.HasTool("harness"))
}

func TestHarnessDoesNotDisplaceTheOrdinaryToolset(t *testing.T) {
	eng := newEngineWithDiagnostics(t, developerRegistry())

	assert.True(t, eng.HasTool("read"))
	assert.True(t, eng.HasTool("bash"))
	assert.True(t, eng.HasTool("plan"), "developer mode observes; it changes no other capability")
}

func TestSubAgentsDoNotInheritTheDeveloperCapability(t *testing.T) {
	child, err := agent.NewEngine(agent.EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: agent.SessionOpts{Cwd: t.TempDir(), ParentID: "parent"},
	})
	require.NoError(t, err)
	assert.False(t, child.HasTool("harness"))
}
