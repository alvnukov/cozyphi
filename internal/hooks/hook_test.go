package hooks

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActionString(t *testing.T) {
	assert.Equal(t, "allow", ActionAllow.String())
	assert.Equal(t, "deny", ActionDeny.String())
	assert.Equal(t, "modify", ActionModify.String())
	assert.Equal(t, "unknown", Action(99).String())
}

func TestFuncHookDefaults(t *testing.T) {
	h := FuncHook{}
	assert.Equal(t, "func", h.Name())
	assert.True(t, h.Match("bash"))
	assert.True(t, h.Match("write"))

	pre, err := h.PreTool(t.Context(), Event{Tool: "bash"})
	require.NoError(t, err)
	assert.Equal(t, ActionAllow, pre.Action)

	post, err := h.PostTool(t.Context(), Event{Tool: "bash"})
	require.NoError(t, err)
	assert.Equal(t, PostResult{}, post)
}

func TestFuncHookPreDenyAndMatch(t *testing.T) {
	h := FuncHook{
		HookName: "guard-bash",
		MatchFn:  MatchTool("bash"),
		Pre: func(_ context.Context, ev Event) (PreResult, error) {
			return PreResult{Action: ActionDeny, Reason: "blocked " + ev.Tool}, nil
		},
	}
	assert.Equal(t, "guard-bash", h.Name())
	assert.True(t, h.Match("bash"))
	assert.False(t, h.Match("write"))

	pre, err := h.PreTool(t.Context(), Event{Tool: "bash"})
	require.NoError(t, err)
	assert.Equal(t, ActionDeny, pre.Action)
	assert.Equal(t, "blocked bash", pre.Reason)
}

func TestFuncHookModifyAndPostContext(t *testing.T) {
	newInput := json.RawMessage(`{"command":"echo ok"}`)
	h := FuncHook{
		HookName: "rewrite",
		Pre: func(_ context.Context, _ Event) (PreResult, error) {
			return PreResult{Action: ActionModify, Input: newInput}, nil
		},
		Post: func(_ context.Context, ev Event) (PostResult, error) {
			return PostResult{Context: "ran " + ev.Tool, Stop: false}, nil
		},
	}

	pre, err := h.PreTool(t.Context(), Event{Input: json.RawMessage(`{}`)})
	require.NoError(t, err)
	assert.Equal(t, ActionModify, pre.Action)
	assert.JSONEq(t, `{"command":"echo ok"}`, string(pre.Input))

	post, err := h.PostTool(t.Context(), Event{Tool: "bash", Output: "ok"})
	require.NoError(t, err)
	assert.Equal(t, "ran bash", post.Context)
	assert.False(t, post.Stop)
}

func TestEventJSONRoundTrip(t *testing.T) {
	ev := Event{
		SessionID: "s1",
		Cwd:       "/tmp/proj",
		Tool:      "bash",
		ToolUseID: "call_1",
		Input:     json.RawMessage(`{"command":"ls"}`),
		Output:    "a\nb",
		Err:       "",
	}
	b, err := json.Marshal(ev)
	require.NoError(t, err)

	var got Event
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, ev.SessionID, got.SessionID)
	assert.Equal(t, ev.Cwd, got.Cwd)
	assert.Equal(t, ev.Tool, got.Tool)
	assert.Equal(t, ev.ToolUseID, got.ToolUseID)
	assert.JSONEq(t, string(ev.Input), string(got.Input))
	assert.Equal(t, ev.Output, got.Output)
}

func TestMatchHelpers(t *testing.T) {
	assert.True(t, MatchTool("bash")("bash"))
	assert.False(t, MatchTool("bash")("write"))
	assert.True(t, MatchAll()("anything"))
}

var _ Hook = FuncHook{}
