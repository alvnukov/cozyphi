package agent

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/lsp"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestLoopPlanGateDenyTeachesBindingRecovery(t *testing.T) {
	var runs atomic.Int32
	server, streams, bodies := fakeContextServer(t, "unused", func(n int32) string {
		switch n {
		case 1:
			return sseToolCallChunk("missing", "lsp", `{"op":"hover","symbol":"needle"}`)
		case 2:
			assert.Zero(t, runs.Load(), "a binding refusal must not execute the tool")
			return sseToolCallChunk("corrected", "lsp", `{"op":"hover","symbol":"needle","plan_step":"inspect"}`)
		default:
			return sseTextChunk()
		}
	})
	engine, err := NewEngine(EngineOpts{
		Model: llm.ModelConfig{
			Name: "fake", BaseURL: server.URL, APIKey: "x", ContextWindow: 100000,
		},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Gate:        permission.AllowAll{},
		MaxRounds:   2,
		// A borrowed LSP callback lets us count dispatches without replacing engine internals.
		LSP: func(context.Context, lsp.Query) (lsp.Result, error) {
			runs.Add(1)
			return lsp.Result{Hover: &lsp.Hover{Text: "matched"}}, nil
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Session().Close()) })
	_, _, _, err = engine.Session().ReplacePlanV2(t.Context(), session.PlanV2{
		Goal: "verify binding recovery", Approach: "exercise provider requests",
		SuccessCriteria: []string{"corrected call executes"},
		Items: []session.PlanItem{
			{
				ID: "inspect", Content: "inspect", Status: session.PlanPending, Type: session.StepExplore,
				Why: "inspect target", DoneWhen: "target inspected",
			},
			{
				ID: "verify", Content: "verify", Status: session.PlanPending, Type: session.StepExplore,
				Why: "verify target", DoneWhen: "target verified",
			},
		},
	}, false)
	require.NoError(t, err)
	_, err = engine.SetPlanApproved(true)
	require.NoError(t, err)
	engine.SetPlanGate(&plangate.Checker{Phase: plangate.PhaseDeny})

	var rejected []session.ToolData
	for ev, err := range engine.Loop(t.Context(), "inspect needle", LoopOpts{}) {
		require.NoError(t, err)
		if td, ok := ev.(session.ToolData); ok && td.Run.Status == session.ToolRejected {
			rejected = append(rejected, td)
		}
	}
	require.Equal(t, int32(1), runs.Load(), "corrected call must run; rejections: %+v", rejected)
	require.Len(t, rejected, 1)
	assert.Contains(t, rejected[0].Run.Output, "plan_step (omitted)")
	assert.NotContains(t, rejected[0].Run.Output, "Pass plan_step", "recovery guidance is model-only")

	require.Equal(t, int32(3), streams.Load())
	requests := bodies()
	require.Len(t, requests, 3)
	// Inspect the actual provider requests, not the engine's stored context.
	toolMessages := func(body string) []llm.Message {
		var request struct {
			Messages []llm.Message `json:"messages"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &request))
		var messages []llm.Message
		for _, message := range request.Messages {
			if message.Role == llm.RoleTool {
				messages = append(messages, message)
			}
		}
		return messages
	}
	msgs := toolMessages(requests[1])
	require.Len(t, msgs, 1)
	assert.True(t, json.Valid([]byte(msgs[0].Content)), "the next provider request carries the structured refusal")
	assert.Contains(t, msgs[0].Content, `"code":"TOOL_NOT_AVAILABLE_IN_CURRENT_PHASE"`)
	assert.Contains(t, msgs[0].Content, "Do not substitute another tool")
	assert.Equal(t, "missing", msgs[0].ToolCallID)
	assert.Contains(t, msgs[0].Content, "plan_step (omitted)")
	assert.Contains(t, msgs[0].Content, "inspect (explore, pending)")
	assert.Contains(t, msgs[0].Content, "verify (explore, pending)")
	assert.Contains(t, msgs[0].Content, "Pass plan_step of one of the compatible steps listed above.")

	msgs = toolMessages(requests[2])
	require.Len(t, msgs, 2)
	assert.Equal(t, "corrected", msgs[1].ToolCallID)
	assert.Contains(t, msgs[1].Content, "matched")
	assert.NotContains(t, msgs[1].Content, "[plan gate]")
}
