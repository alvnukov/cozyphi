package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

func TestExecutorPlanGateDenyTeachesBindingRecovery(t *testing.T) {
	var runs int
	reg := tools.Registry{
		"grep": {
			Definition: llm.ToolDefinition{Name: "grep"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				runs++
				return tools.Result{Content: "matched"}, nil
			},
		},
	}
	plan := session.Plan{Revision: 1, Approved: true, Items: []session.PlanItem{
		{ID: "inspect", Content: "inspect", Status: session.PlanInProgress, Type: session.StepExplore},
		{ID: "verify", Content: "verify", Status: session.PlanPending, Type: session.StepExplore},
	}}
	ex := NewExecutor(reg, permission.AllowAll{}, nil, nil)
	ex.SetPlanGate(
		&plangate.Checker{Phase: plangate.PhaseDeny},
		func() session.Plan { return plan },
		nil,
		nil,
		nil,
		nil,
	)

	var rejected session.ToolData
	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "missing",
		Function: llm.Function{Name: "grep", Arguments: `{"pattern":"needle","path":"."}`},
	}}, func(td session.ToolData) bool {
		if td.Run.Status == session.ToolRejected {
			rejected = td
		}
		return true
	})
	require.Zero(t, runs, "a binding refusal must not execute the tool")
	require.Len(t, msgs, 1)
	assert.Equal(t, llm.RoleTool, msgs[0].Role)
	assert.Equal(t, "missing", msgs[0].ToolCallID)
	assert.Contains(t, msgs[0].Content, "plan_step (omitted)")
	assert.Contains(t, msgs[0].Content, "inspect (explore, in_progress)")
	assert.Contains(t, msgs[0].Content, "verify (explore, pending)")
	assert.Contains(t, msgs[0].Content, "Pass plan_step of one of the compatible steps listed above.")
	assert.Contains(t, rejected.Run.Output, "plan_step (omitted)")
	assert.NotContains(t, rejected.Run.Output, "Pass plan_step", "recovery guidance is model-only")

	msgs, _, _ = ex.run(t.Context(), []llm.ToolCall{{
		ID:       "corrected",
		Function: llm.Function{Name: "grep", Arguments: `{"pattern":"needle","path":".","plan_step":"inspect"}`},
	}}, func(session.ToolData) bool { return true })
	require.Equal(t, 1, runs, "correcting only the binding must allow the same tool to run")
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "matched")
	assert.NotContains(t, msgs[0].Content, "[plan gate]")
}
