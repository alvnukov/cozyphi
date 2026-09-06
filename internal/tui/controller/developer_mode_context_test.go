package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

func contextField(t *testing.T, c *Controller, key string) diag.Field {
	t.Helper()
	explained, err := c.diagnostics.Explain(t.Context(), diag.CategoryContext, key)
	require.NoError(t, err)
	return explained.Field
}

func TestTheHarnessReportsTheWindowThisSessionActuallyBudgetsAgainst(t *testing.T) {
	c := developerToolRuntime(t)

	window := contextField(t, c, diag.KeyContextWindow)
	assert.Equal(t, int64(c.engineRef.Load().ContextWindow()), window.Effective.Value.Int,
		"the effective layer is the number the engine budgets against")
	assert.Contains(t, window.Effective.Source.Ref, "narrowing")

	tokens := contextField(t, c, diag.KeyContextTokens)
	assert.Equal(t, diag.StatePresent, tokens.Effective.State)
	assert.Equal(t, diag.TokenSourceEstimate,
		contextField(t, c, diag.KeyContextTokenSource).Effective.Value.Str,
		"no provider has counted this context, and the harness says so rather than "+
			"presenting a heuristic as a measurement")
}

func TestTheCompactionPolicyIsExplainedAsConfiguredAndAsItActs(t *testing.T) {
	c := developerToolRuntime(t)

	threshold := contextField(t, c, diag.KeyContextCompactThreshold)
	assert.Equal(t, diag.StateUnset, threshold.Configured.State,
		"this session set no reminder threshold of its own")
	assert.Equal(t, diag.StatePresent, threshold.Effective.State,
		"and one is still derived from the window it runs in")

	assert.True(t, contextField(t, c, diag.KeyContextCompactionEnabled).Effective.Value.Bool)
	assert.Equal(t, diag.StateUnset,
		contextField(t, c, diag.KeyContextLastCompactionKind).Effective.State,
		"a fresh session carries no compaction, which is an answer and not a zero")
}

func TestThePromptSourcesAreReportedAsCountsAndScopes(t *testing.T) {
	c := developerToolRuntime(t)

	assert.Positive(t, contextField(t, c, diag.KeyContextPromptBytes).Effective.Value.Int,
		"the prompt this session carries was measured where it was assembled")

	scopes := contextField(t, c, diag.KeyContextInstructionScopes)
	require.Equal(t, diag.StatePresent, scopes.Effective.State)
	for _, scope := range scopes.Effective.Value.List {
		assert.Contains(t, []string{"agent_dir", "ancestor", "workspace"}, scope,
			"a scope says how far a file's authority reaches, never where it lives")
	}
}

func TestObservingTheContextChangesNothingAboutIt(t *testing.T) {
	c := developerToolRuntime(t)
	engine := c.engineRef.Load()
	before := engine.ContextObservation()
	entries := len(engine.Session().PathEntries())

	for range 3 {
		contextField(t, c, diag.KeyContextTokens)
	}

	after := engine.ContextObservation()
	assert.Len(t, engine.Session().PathEntries(), entries,
		"an observation appends nothing to the session")
	assert.Equal(t, before.Compaction, after.Compaction,
		"and it compacts nothing, trims nothing and schedules nothing")
	assert.Equal(t, before.Sources, after.Sources,
		"nor does it re-read an instruction file, a skill or a memory")
}

func TestTheContextAnswerCarriesNoConversation(t *testing.T) {
	c := developerToolRuntime(t)

	snapshot, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryContext)
	require.NoError(t, err)
	rendered, err := json.Marshal(snapshot)
	require.NoError(t, err)

	body := string(rendered)
	assert.NotContains(t, body, c.cwd, "not even the workspace path rides in this category")
	assert.NotContains(t, body, "AGENTS.md")
	assert.NotContains(t, body, "preview")
}

func TestNoEngineYetLeavesTheContextCategoryUnavailable(t *testing.T) {
	c := &Controller{}
	registry := diag.NewRegistry(nil, diag.DefaultLimits(),
		diag.NewContextCollector(diag.ContextDeps{State: c.contextState}))

	explained, err := registry.Explain(t.Context(), diag.CategoryContext, diag.KeyContextWindow)
	require.NoError(t, err)
	assert.Equal(t, diag.StateUnavailable, explained.Field.Effective.State)
}
