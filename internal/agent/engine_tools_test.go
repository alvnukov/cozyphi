package agent_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tools"
)

func newReadonlyEngine(t *testing.T) *agent.Engine {
	t.Helper()
	eng, err := agent.NewEngine(agent.EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: agent.SessionOpts{Cwd: t.TempDir()},
		Tools:       tools.ReadonlyTools(),
	})
	require.NoError(t, err)
	return eng
}

func TestSetModelKeepsCustomTools(t *testing.T) {
	eng := newReadonlyEngine(t)
	require.False(t, eng.HasTool("write"))

	err := eng.SetModel(llm.ModelConfig{Name: "fake2", BaseURL: "http://127.0.0.1:9", APIKey: "x"})
	require.NoError(t, err)

	assert.True(t, eng.HasTool("read"))
	assert.True(t, eng.HasTool("bash"))
	assert.False(t, eng.HasTool("write"), "SetModel must not widen a readonly engine to DefaultTools")
	assert.False(t, eng.HasTool("edit"), "SetModel must not widen a readonly engine to DefaultTools")
}

func TestSetJobsKeepsCustomTools(t *testing.T) {
	eng := newReadonlyEngine(t)
	require.False(t, eng.HasTool("write"))

	eng.SetJobs(nil)

	assert.True(t, eng.HasTool("read"))
	assert.False(t, eng.HasTool("write"), "SetJobs must not widen a readonly engine to DefaultTools")
	assert.False(t, eng.HasTool("edit"), "SetJobs must not widen a readonly engine to DefaultTools")
}

func TestPlanToolOnlyBelongsToPrimaryDefaultEngine(t *testing.T) {
	primary, err := agent.NewEngine(agent.EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: agent.SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)
	assert.True(t, primary.HasTool("plan"))

	custom := newReadonlyEngine(t)
	assert.False(t, custom.HasTool("plan"), "custom and readonly engines must not gain parent plan access")

	child, err := agent.NewEngine(agent.EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: agent.SessionOpts{Cwd: t.TempDir(), ParentID: "parent"},
	})
	require.NoError(t, err)
	assert.False(t, child.HasTool("plan"), "sub-agents must not access the parent-visible plan")
}
