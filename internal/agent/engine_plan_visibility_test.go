package agent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

// providerToolNames parses the tool names out of one captured request body —
// the provider projection, not the executor registry.
func providerToolNames(t *testing.T, body string) map[string]bool {
	t.Helper()
	var request struct {
		Tools []struct {
			Function *struct {
				Name string `json:"name"`
			} `json:"function"`
			Name string `json:"name"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &request))
	names := make(map[string]bool, len(request.Tools))
	for _, tool := range request.Tools {
		if tool.Function != nil {
			names[tool.Function.Name] = true
			continue
		}
		names[tool.Name] = true
	}
	return names
}

// TestEngineShowsOnlyToolsThePlanStatePermits pins provider-facing tool
// visibility: in useplan the schemas the provider sees match the plan gate.
// An unapproved plan shows exempt tools only; an approved plan shows exactly
// the union its in_progress steps allow; and the executor registry keeps the
// full set so a hallucinated hidden tool still gets the gate's reason.
func TestEngineShowsOnlyToolsThePlanStatePermits(t *testing.T) {
	server, bodies := capturingTextServer(t)
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		AutoApprove: func() bool { return true },
	})
	require.NoError(t, err)

	// Fresh session: useplan defaults to deny, so gateable schemas never leave.
	drain(t, engine, "hello")
	sent := bodies()
	require.NotEmpty(t, sent)
	names := providerToolNames(t, sent[len(sent)-1])
	assert.True(t, names["plan"] && names["context"], "exempt tools stay visible")
	for _, hidden := range []string{"read", "edit", "bash", "agent_spawn"} {
		assert.False(t, names[hidden], "%s must be hidden while the plan is unapproved", hidden)
	}

	// Approved explore step: the read-only set appears, write stays hidden.
	// (lsp rides on an attached LSP query func, absent here — the policy test
	// covers its visibility.)
	_, err = engine.updatePlan(t.Context(), []session.PlanItem{{
		Content: "look around", Status: session.PlanInProgress, Type: session.StepExplore,
	}})
	require.NoError(t, err)
	drain(t, engine, "explore")
	sent = bodies()
	names = providerToolNames(t, sent[len(sent)-1])
	for _, name := range []string{"read", "grep", "find", "ls"} {
		assert.True(t, names[name], "%s must be visible on an explore step", name)
	}
	assert.False(t, names["edit"], "edit must wait for a step that allows it")
	assert.False(t, names["bash"], "bash must wait for a run step")

	// The explore step completes, an edit step goes active: write/edit appear
	// (with read still inherited), bash stays hidden.
	_, err = engine.updatePlan(t.Context(), []session.PlanItem{
		{Content: "look around", Status: session.PlanCompleted, Type: session.StepExplore},
		{Content: "change files", Status: session.PlanInProgress, Type: session.StepEdit},
	})
	require.NoError(t, err)
	drain(t, engine, "edit")
	sent = bodies()
	names = providerToolNames(t, sent[len(sent)-1])
	assert.True(t, names["read"] && names["write"] && names["edit"], "rank inheritance keeps read available")
	assert.False(t, names["bash"], "bash must stay hidden on an edit step")

	// Defense in depth: the executor registry still resolves hidden tools, so
	// a call hallucinated from an earlier round reaches the plan gate.
	assert.True(t, engine.HasTool("bash"), "the executor registry keeps the full set")
}
