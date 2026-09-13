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

// Plan state changes execution rights, never the provider's tool catalog.
func TestEngineKeepsToolCatalogAcrossPlanStates(t *testing.T) {
	server, bodies := capturingTextServer(t)
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		AutoApprove: func() bool { return true },
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Session().Close()) })

	// Even before approval the model sees the tools it will eventually use.
	drain(t, engine, "hello")
	sent := bodies()
	require.NotEmpty(t, sent)
	names := providerToolNames(t, sent[len(sent)-1])
	assert.True(t, names["plan"] && names["context"], "exempt tools stay visible")
	for _, name := range []string{"read", "write", "edit", "bash"} {
		assert.True(t, names[name], "%s must remain known before approval", name)
	}
	initial := names
	assert.False(t, names["agent_spawn"], "an unattached capability must not be invented")

	// Approval and a read-only step keep the same catalog.
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
	assert.Equal(t, initial, names)

	// Completing one step and starting another must not remove schemas.
	_, err = engine.updatePlan(t.Context(), []session.PlanItem{
		{Content: "look around", Status: session.PlanCompleted, Type: session.StepExplore},
		{Content: "change files", Status: session.PlanInProgress, Type: session.StepEdit},
	})
	require.NoError(t, err)
	drain(t, engine, "edit")
	sent = bodies()
	names = providerToolNames(t, sent[len(sent)-1])
	assert.True(t, names["read"] && names["write"] && names["edit"], "rank inheritance keeps read available")
	assert.Equal(t, initial, names)

	assert.True(t, engine.HasTool("bash"), "the executor registry keeps the full set")
	engine.SetMode(ModePlan)
	drain(t, engine, "draft")
	sent = bodies()
	assert.Equal(t, initial, providerToolNames(t, sent[len(sent)-1]))
	assert.False(t, engine.HasTool("write"), "plan mode still has no write handler")
	engine.SetMode(ModeBuild)
	drain(t, engine, "build")
	sent = bodies()
	assert.Equal(t, initial, providerToolNames(t, sent[len(sent)-1]))
}
