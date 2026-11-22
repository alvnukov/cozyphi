package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

func TestCodeReadGroupOverlapAndTrace(t *testing.T) {
	const groupSize = 8
	readGroup := json.RawMessage(
		`{"calls":[` + strings.Repeat(`{"name":"ls","args":{}},`, groupSize-1) + `{"name":"ls","args":{}}]}`,
	)
	var active, entered, completed atomic.Int32
	release := [2]chan struct{}{make(chan struct{}), make(chan struct{})}
	tool := tools.Tool{
		Definition:     llm.ToolDefinition{Name: "ls"},
		ConcurrentRead: func(json.RawMessage) bool { return true },
		Run: func(ctx context.Context, _ json.RawMessage) (tools.Result, error) {
			n := entered.Add(1)
			current := active.Add(1)
			defer active.Add(-1)
			wave := (n - 1) / concurrentReadLimit
			if current == concurrentReadLimit {
				close(release[wave])
			}
			select {
			case <-ctx.Done():
				return tools.Result{}, ctx.Err()
			case <-release[wave]:
			}
			completed.Add(1)
			return tools.Result{Structured: true}, nil
		},
	}
	e := codeExecutor(func(ctx context.Context, call codeTestHost) (CodeResult, error) {
		values, err := call(ctx, "cozy:parallel_read", readGroup)
		require.NoError(t, err)
		for _, item := range values.([]joinedResult) {
			require.Empty(t, item.Error)
			require.Equal(t, true, item.Value)
		}
		return CodeResult{Output: "done"}, nil
	}, tool, nil)
	var trace []session.ToolRun
	e.saveCodeTrace = func(run session.ToolRun) error {
		trace = append(trace, run)
		return nil
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	msg, _, _ := e.run(ctx, []llm.ToolCall{codeTestCall()}, func(session.ToolData) bool { return true })
	require.Equal(t, "done", msg[0].Content)
	require.EqualValues(t, groupSize, completed.Load())
	require.Len(t, trace, groupSize*2)
	require.Equal(t, session.ToolInProgress, trace[0].Status)
}

func TestCodeReadGroupHookStopPrecedesNextDispatch(t *testing.T) {
	var count atomic.Int32
	tool := tools.Tool{
		Definition: llm.ToolDefinition{Name: "find"}, ConcurrentRead: func(json.RawMessage) bool { return true },
		Run: func(context.Context, json.RawMessage) (tools.Result, error) {
			count.Add(1)
			return tools.Result{Content: "visible"}, nil
		},
	}
	h := hooks.NewManager(hooks.Entry{Kind: hooks.KindPostTool, Hook: hooks.FuncHook{
		HookName: "stop",
		MatchFn:  hooks.MatchTool("find"),
		Post: func(context.Context, hooks.Event) (hooks.PostResult, error) {
			return hooks.PostResult{Stop: true, Reason: "halt"}, nil
		},
	}})
	e := codeExecutor(func(ctx context.Context, call codeTestHost) (CodeResult, error) {
		_, err := call(ctx, "cozy:parallel_read", json.RawMessage(`{"calls":[{"name":"write","args":{}}]}`))
		require.Error(t, err)
		output, err := call(
			ctx,
			"cozy:parallel_read",
			json.RawMessage(`{"calls":[{"name":"find","args":{}},{"name":"find","args":{}}]}`),
		)
		require.NoError(t, err)
		require.NotEmpty(t, output.([]joinedResult)[1].Error)
		return CodeResult{Output: "done"}, nil
	}, tool, h)
	msg, _, stop := e.run(t.Context(), []llm.ToolCall{codeTestCall()}, func(session.ToolData) bool { return true })
	require.Equal(t, "halt", stop.Reason())
	require.EqualValues(t, 1, count.Load())
	require.Contains(t, msg[0].Content, "halt")
}
