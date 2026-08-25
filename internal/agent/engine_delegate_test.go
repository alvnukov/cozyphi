package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
)

func newJobsEngine(t *testing.T, serverURL string, jobs *job.Manager) *Engine {
	t.Helper()
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: serverURL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Jobs:        jobs,
	})
	require.NoError(t, err)
	return engine
}

func newTestJobManager(t *testing.T) *job.Manager {
	t.Helper()
	mgr, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(_ context.Context, _ job.RunEnv) (string, error) {
			return "ok", nil
		}),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(t.Context()) })
	return mgr
}

func lastBody(t *testing.T, bodies func() []string) string {
	t.Helper()
	all := bodies()
	require.NotEmpty(t, all)
	return all[len(all)-1]
}

// TestLoopDelegatesLeadingAgentMention pins the composer @agent contract: a
// prompt addressed "@role task" is rewritten into an explicit agent_spawn
// delegation so the sub-agent pipeline renders it like any other spawn.
func TestLoopDelegatesLeadingAgentMention(t *testing.T) {
	server, bodies := capturingTextServer(t)
	engine := newJobsEngine(t, server.URL, newTestJobManager(t))

	drainLoop(t, engine, "@explore find the sink for X")
	req := lastBody(t, bodies)
	require.Contains(t, req, "addressed this message")
	require.Contains(t, req, `"explore"`)
	require.Contains(t, req, "find the sink for X")
}

// TestLoopDelegationRequiresTextAfterRole: a bare "@explore" (no task) is not
// a delegation and must reach the model verbatim.
func TestLoopDelegationRequiresTextAfterRole(t *testing.T) {
	server, bodies := capturingTextServer(t)
	engine := newJobsEngine(t, server.URL, newTestJobManager(t))

	drainLoop(t, engine, "@explore")
	require.NotContains(t, lastBody(t, bodies), "addressed this message")
}

// TestLoopNoDelegationWithoutJobs: with sub-agents disabled there is no
// agent_spawn tool, so a leading @role must stay plain prompt text.
func TestLoopNoDelegationWithoutJobs(t *testing.T) {
	server, bodies := capturingTextServer(t)
	engine := newJobsEngine(t, server.URL, nil)

	drainLoop(t, engine, "@worker fix the tests")
	require.NotContains(t, lastBody(t, bodies), "addressed this message")
}

// TestSplitDelegationPrefix covers boundary parsing without a server.
func TestSplitDelegationPrefix(t *testing.T) {
	cases := []struct {
		in   string
		role string
		rest string
		ok   bool
	}{
		{"@explore find x", "explore", "find x", true},
		{"  @worker  do it", "worker", "do it", true},
		{"@review\ncheck", "review", "check", true},
		{"@explore", "", "", false},
		{"@workere typo", "", "", false},
		{"@unknown find", "", "", false},
		{"hello @explore find", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		role, rest, ok := splitDelegationPrefix(tc.in)
		require.Equal(t, tc.ok, ok, tc.in)
		require.Equal(t, tc.role, role, tc.in)
		require.Equal(t, tc.rest, rest, tc.in)
	}
}
