package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pulseaiclub/phi/internal/llm"
)

func TestBuildRequestSystemIsArray(t *testing.T) {
	cfg := llm.ModelConfig{Name: "claude-sonnet-4-20250514", APIKey: "k", BaseURL: "https://api.anthropic.com"}
	req := BuildRequest(cfg, "You are helpful.", []llm.Message{
		{Role: llm.RoleUser, Content: "Hi"},
	}, nil)

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	systemRaw, ok := raw["system"]
	if !ok {
		t.Fatal("expected system field in request body")
	}
	var asBlocks []sysBlock
	if err := json.Unmarshal(systemRaw, &asBlocks); err != nil {
		t.Fatalf("system must be an array, got: %s", string(systemRaw))
	}
	if len(asBlocks) != 1 {
		t.Fatalf("expected 1 system block, got %d", len(asBlocks))
	}
	if asBlocks[0].Type != "text" || !strings.Contains(asBlocks[0].Text, "helpful") {
		t.Fatalf("unexpected system block: %+v", asBlocks[0])
	}
	if asBlocks[0].CacheControl == nil {
		t.Fatal("expected cache_control on system block")
	}
}

func TestBuildRequestMergesToolResults(t *testing.T) {
	cfg := llm.ModelConfig{Name: "claude-sonnet-4-20250514", APIKey: "k", BaseURL: "https://api.anthropic.com"}
	req := BuildRequest(cfg, "", []llm.Message{
		{Role: llm.RoleUser, Content: "run tools"},
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{ID: "call_01", Function: llm.Function{Name: "read", Arguments: `{"path":"a.go"}`}},
				{ID: "call_02", Function: llm.Function{Name: "read", Arguments: `{"path":"b.go"}`}},
				{ID: "call_03", Function: llm.Function{Name: "read", Arguments: `{"path":"c.go"}`}},
			},
		},
		{Role: llm.RoleTool, ToolCallID: "call_01", Content: "a"},
		{Role: llm.RoleTool, ToolCallID: "call_02", Content: "b"},
		{Role: llm.RoleTool, ToolCallID: "call_03", Content: "c"},
	}, nil)

	if len(req.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(req.Messages))
	}

	results, ok := req.Messages[2].Content.([]anthropicContentBlock)
	if !ok {
		t.Fatalf("expected tool results as content blocks, got %T", req.Messages[2].Content)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 merged tool_result blocks, got %d", len(results))
	}
	for i, wantID := range []string{"call_01", "call_02", "call_03"} {
		if results[i].Type != "tool_result" {
			t.Fatalf("block %d: expected tool_result, got %s", i, results[i].Type)
		}
		if results[i].ToolUseID != wantID {
			t.Fatalf("block %d: expected tool_use_id %s, got %s", i, wantID, results[i].ToolUseID)
		}
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Count(string(body), `"tool_result"`) != 3 {
		t.Fatalf("expected 3 tool_result entries in JSON, got: %s", string(body))
	}
	if strings.Count(string(body), `"role":"user"`) != 2 {
		t.Fatalf("expected 2 user messages (prompt + tool results), got: %s", string(body))
	}
}

func TestBuildRequestToolsSchema(t *testing.T) {
	cfg := llm.ModelConfig{Name: "claude-sonnet-4-20250514", APIKey: "k", BaseURL: "https://api.anthropic.com"}
	tools := []llm.ToolDefinition{{
		Name:        "read",
		Description: "read a file",
		Params: &llm.FunctionParameters{
			Type:       "object",
			Properties: llm.Object{"path": llm.Object{"type": "string"}},
			Required:   []string{"path"},
		},
	}}

	req := BuildRequest(cfg, "", []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, tools)
	if len(req.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(req.Tools))
	}
	if req.Tools[0].Name != "read" {
		t.Fatalf("unexpected tool name: %s", req.Tools[0].Name)
	}
	if !strings.Contains(string(req.Tools[0].InputSchema), `"required"`) {
		t.Fatalf("input_schema missing required list: %s", string(req.Tools[0].InputSchema))
	}
	if req.Tools[0].CacheControl == nil {
		t.Fatal("expected cache_control on last tool")
	}
}

func TestProcessStreamTextAndUsage(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":12,"cache_read_input_tokens":900,"cache_creation_input_tokens":0}}}`,
		"",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		"",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		"",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
		"",
		`data: {"type":"message_delta","usage":{"output_tokens":7}}`,
		"",
		`data: {"type":"message_stop"}`,
		"",
	}, "\n")

	events := processForTest(sse)

	var text strings.Builder
	var done *llm.StreamEvent
	for _, ev := range events {
		if ev.Type == llm.StreamEventTypeError {
			t.Fatalf("stream error: %s", ev.Err)
		}
		switch ev.Type {
		case llm.StreamEventTypeDelta:
			_, _ = text.WriteString(ev.Delta.Content)
		case llm.StreamEventTypeDone:
			done = &ev
		}
	}

	if text.String() != "Hello world" {
		t.Fatalf("expected delta text 'Hello world', got %q", text.String())
	}
	if done == nil {
		t.Fatal("expected done event")
	}
	msg := done.Partial.Choices[0].Message
	if msg.Content != "Hello world" {
		t.Fatalf("expected content 'Hello world', got %q", msg.Content)
	}
	if done.Partial.Usage.PromptTokens != 12 || done.Partial.Usage.CompletionTokens != 7 ||
		done.Partial.Usage.TotalTokens != 19 {
		t.Fatalf("unexpected usage: %+v", done.Partial.Usage)
	}
	if done.Partial.Usage.CachedTokens() != 900 {
		t.Fatalf("expected cached tokens 900, got %d", done.Partial.Usage.CachedTokens())
	}
}

func TestProcessStreamToolUseAndThinking(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":5}}}`,
		"",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"let me think"}}`,
		"",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_01","name":"read"}}`,
		"",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}`,
		"",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"a.go\"}"}}`,
		"",
		`data: {"type":"content_block_stop","index":0}`,
		"",
		`data: {"type":"message_stop"}`,
		"",
	}, "\n")

	events := processForTest(sse)

	var thinking, args string
	var done *llm.StreamEvent
	for _, ev := range events {
		if ev.Type == llm.StreamEventTypeError {
			t.Fatalf("stream error: %s", ev.Err)
		}
		switch ev.Type {
		case llm.StreamEventTypeDelta:
			thinking += ev.Delta.ReasoningContent
			for _, tc := range ev.Delta.ToolCalls {
				args = tc.Function.Arguments
			}
		case llm.StreamEventTypeDone:
			done = &ev
		}
	}

	if thinking != "let me think" {
		t.Fatalf("expected thinking 'let me think', got %q", thinking)
	}
	if done == nil {
		t.Fatal("expected done event")
	}
	msg := done.Partial.Choices[0].Message
	if msg.ReasoningContent != "let me think" {
		t.Fatalf("expected reasoning in final message, got %q", msg.ReasoningContent)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	tc := msg.ToolCalls[0]
	if tc.ID != "toolu_01" || tc.Function.Name != "read" {
		t.Fatalf("unexpected tool call: %+v", tc)
	}
	if tc.Function.Arguments != `{"path":"a.go"}` {
		t.Fatalf("expected accumulated args, got %q", tc.Function.Arguments)
	}
	if args != `{"path":"a.go"}` {
		t.Fatalf("expected delta tool args, got %q", args)
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "https://api.anthropic.com/v1"},
		{"https://api.anthropic.com", "https://api.anthropic.com/v1"},
		{"https://api.anthropic.com/", "https://api.anthropic.com/v1"},
		{"https://api.anthropic.com/v1", "https://api.anthropic.com/v1"},
		{"http://localhost:8080", "http://localhost:8080/v1"},
	}
	for _, c := range cases {
		if got := normalizeBaseURL(c.in); got != c.want {
			t.Fatalf("normalizeBaseURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// processForTest runs processStream and returns the yielded events.
func processForTest(sse string) []llm.StreamEvent {
	var events []llm.StreamEvent
	processStream(strings.NewReader(sse), func(ev llm.StreamEvent, err error) bool {
		if err != nil {
			events = append(events, llm.StreamEvent{Type: llm.StreamEventTypeError, Err: err.Error()})
			return false
		}
		events = append(events, ev)
		return true
	})
	return events
}
