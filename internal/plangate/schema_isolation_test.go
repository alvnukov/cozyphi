package plangate_test

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

func TestInjectPlanStepLeavesInputUntouched(t *testing.T) {
	// Spare capacity exposes writes through the original Required backing array.
	required := []string{"path", "reserved"}
	params := &llm.FunctionParameters{
		Type:       "object",
		Properties: llm.Object{"path": llm.Object{"type": "string"}},
		Required:   required[:1],
	}
	input := []tooldef.Tool{{Definition: llm.ToolDefinition{Name: "read", Params: params}}}

	out := plangate.InjectPlanStep(input)

	require.Len(t, out, 1)
	assert.Equal(t, "read", out[0].Definition.Name)
	assert.Equal(t, "object", out[0].Definition.Params.Type)
	assert.Equal(t, llm.Object{"type": "string"}, out[0].Definition.Params.Properties["path"])
	assert.Contains(t, out[0].Definition.Params.Properties, "plan_step")
	assert.Equal(t, []string{"path", "plan_step"}, out[0].Definition.Params.Required)
	assert.Equal(t, llm.Object{"path": llm.Object{"type": "string"}}, params.Properties)
	assert.Equal(t, []string{"path"}, params.Required)
	assert.Equal(t, []string{"path", "reserved"}, required)
}

func TestInjectPlanStepIsolatesPolicyOutputs(t *testing.T) {
	voluntary, err := plangate.Compile(plangate.Defaults{
		Types:                []plangate.TypeDefaults{{Name: "work", Tools: []string{"read"}}},
		AdditionalExemptions: []string{"lsp"},
	})
	require.NoError(t, err)
	required, err := plangate.Compile(plangate.DefaultDefaults())
	require.NoError(t, err)

	for _, requiredFirst := range []bool{false, true} {
		name := "voluntary-first"
		if requiredFirst {
			name = "required-first"
		}
		t.Run(name, func(t *testing.T) {
			params := &llm.FunctionParameters{
				Type:       "object",
				Properties: llm.Object{"query": llm.Object{"type": "string"}},
				Required:   []string{"query"},
			}
			// Even two tools sharing one schema must receive independent bindings.
			input := []tooldef.Tool{
				{Definition: llm.ToolDefinition{Name: "lsp", Params: params}},
				{Definition: llm.ToolDefinition{Name: "read", Params: params}},
			}
			var optional, mandatory []tooldef.Tool
			if requiredFirst {
				mandatory = required.InjectPlanStep(input)
				optional = voluntary.InjectPlanStep(input)
			} else {
				optional = voluntary.InjectPlanStep(input)
				mandatory = required.InjectPlanStep(input)
			}

			assert.Equal(t, []string{"query"}, optional[0].Definition.Params.Required)
			assert.Contains(t, optional[0].Definition.Params.Properties, "plan_step")
			assert.Equal(t, []string{"query", "plan_step"}, optional[1].Definition.Params.Required)
			assert.Equal(t, []string{"query", "plan_step"}, mandatory[0].Definition.Params.Required)
			assert.Equal(t, []string{"query", "plan_step"}, mandatory[1].Definition.Params.Required)
			assert.Equal(t, []string{"query"}, params.Required)
			assert.NotContains(t, params.Properties, "plan_step")

			mandatory[0].Definition.Params.Properties["private"] = llm.Object{"type": "boolean"}
			mandatory[0].Definition.Params.Required[0] = "private"
			assert.NotContains(t, optional[0].Definition.Params.Properties, "private")
			assert.NotContains(t, mandatory[1].Definition.Params.Properties, "private")
			assert.NotContains(t, params.Properties, "private")
			assert.Equal(t, []string{"query"}, optional[0].Definition.Params.Required)
			assert.Equal(t, []string{"query", "plan_step"}, mandatory[1].Definition.Params.Required)
			assert.Equal(t, []string{"query"}, params.Required)
		})
	}
}

func TestInjectPlanStepHandlesNilSchemas(t *testing.T) {
	voluntary, err := plangate.Compile(plangate.Defaults{
		AdditionalExemptions: []string{"lsp"},
	})
	require.NoError(t, err)

	for _, nilParams := range []bool{false, true} {
		name := "nil-properties"
		if nilParams {
			name = "nil-params"
		}
		t.Run(name, func(t *testing.T) {
			var params *llm.FunctionParameters
			if !nilParams {
				params = &llm.FunctionParameters{Type: "object"}
			}
			input := []tooldef.Tool{
				{Definition: llm.ToolDefinition{Name: "read", Params: params}},
				{Definition: llm.ToolDefinition{Name: "lsp", Params: params}},
				{Definition: llm.ToolDefinition{Name: "question", Params: params}},
			}

			out := voluntary.InjectPlanStep(input)

			for _, tool := range out[:2] {
				require.NotNil(t, tool.Definition.Params)
				assert.Equal(t, "object", tool.Definition.Params.Type)
				assert.Contains(t, tool.Definition.Params.Properties, "plan_step")
			}
			assert.Equal(t, []string{"plan_step"}, out[0].Definition.Params.Required)
			assert.Empty(t, out[1].Definition.Params.Required)
			assert.Equal(t, input[2].Definition, out[2].Definition, "mandatory exemptions stay untouched")
			for _, tool := range input {
				if nilParams {
					assert.Nil(t, tool.Definition.Params)
				} else {
					assert.Nil(t, tool.Definition.Params.Properties)
					assert.Nil(t, tool.Definition.Params.Required)
				}
			}
		})
	}
}

func TestInjectPlanStepPreservesExistingBinding(t *testing.T) {
	binding := llm.Object{"type": "string", "description": "custom binding"}
	input := []tooldef.Tool{{Definition: llm.ToolDefinition{
		Name: "read",
		Params: &llm.FunctionParameters{
			Type:       "object",
			Properties: llm.Object{"plan_step": binding},
			Required:   []string{"plan_step"},
		},
	}}}

	out := plangate.InjectPlanStep(plangate.InjectPlanStep(input))

	assert.Equal(t, binding, out[0].Definition.Params.Properties["plan_step"])
	assert.Equal(t, []string{"plan_step"}, out[0].Definition.Params.Required)
}

func TestInjectPlanStepConcurrentMarshal(t *testing.T) {
	input := []tooldef.Tool{{Definition: llm.ToolDefinition{
		Name: "read",
		Params: &llm.FunctionParameters{
			Type:       "object",
			Properties: llm.Object{"path": llm.Object{"type": "string"}},
			Required:   []string{"path"},
		},
	}}}
	inFlight := plangate.InjectPlanStep(input)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			<-start
			for range 100 {
				out := plangate.InjectPlanStep(input)
				for _, tools := range [][]tooldef.Tool{input, inFlight, out} {
					_, err := json.Marshal(tools[0].Definition)
					assert.NoError(t, err)
				}
			}
		})
	}
	close(start)
	workers.Wait()
}
