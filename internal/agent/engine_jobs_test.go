package agent_test

import (
	"context"
	"testing"

	"github.com/pulseaiclub/phi/internal/agent"
	"github.com/pulseaiclub/phi/internal/job"
	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEngineRegistersJobs(t *testing.T) {
	mgr, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(ctx context.Context, env job.RunEnv) (string, error) {
			return "ok", nil
		}),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(context.Background()) })

	eng, err := agent.NewEngine(agent.EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: agent.SessionOpts{Cwd: t.TempDir()},
		Jobs:        mgr,
	})
	require.NoError(t, err)
	assert.Same(t, mgr, eng.Jobs())

	require.NoError(t, eng.SetModel(llm.ModelConfig{Name: "fake2", BaseURL: "http://127.0.0.1:9", APIKey: "x"}))
	assert.Same(t, mgr, eng.Jobs())
}

func TestChildToolsHaveNoAgent(t *testing.T) {
	for _, tool := range agent.ChildTools() {
		assert.NotContains(t, tool.Definition.Name, "agent_")
	}
}
