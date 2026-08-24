package responses_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/llm/responses"
)

func TestStreamStopsImmediatelyWhenConsumerBreaks(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"first\"}\n\n")
	}))
	defer server.Close()

	events := responses.Stream(t.Context(), server.Client(), llm.ModelConfig{
		Name: "test", BaseURL: server.URL,
	}, nil, nil, "")
	for event, err := range events {
		require.NoError(t, err)
		require.Equal(t, llm.StreamEventTypeDelta, event.Type)
		break
	}
}

func TestStreamHTTPErrorDoesNotEchoResponseBody(t *testing.T) {
	t.Parallel()

	const reflectedSecret = "secret-must-not-escape"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, reflectedSecret, http.StatusUnauthorized)
	}))
	defer server.Close()

	events := responses.Stream(t.Context(), server.Client(), llm.ModelConfig{
		Name: "test", BaseURL: server.URL,
	}, nil, nil, "")
	for _, err := range events {
		require.Error(t, err)
		require.NotContains(t, err.Error(), reflectedSecret)
	}
}

func TestStreamNormalizesResponsesProtocol(t *testing.T) {
	t.Parallel()

	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/responses", r.URL.Path)
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &request))

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(
			w,
			"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"+
				"event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"output_index\":1,\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\\\"README.md\\\"}\"}}\n\n"+
				"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":1,\"status\":\"completed\",\"model\":\"test\",\"output\":[],\"parallel_tool_calls\":true,\"tool_choice\":\"auto\",\"tools\":[],\"usage\":{\"input_tokens\":11,\"input_tokens_details\":{\"cached_tokens\":3},\"output_tokens\":5,\"output_tokens_details\":{\"reasoning_tokens\":0},\"total_tokens\":16}}}\n\n",
		)
	}))
	defer server.Close()

	cfg := llm.ModelConfig{
		Name:            "test",
		Protocol:        llm.ProtocolOpenAIResponses,
		APIKey:          "secret",
		BaseURL:         server.URL,
		ReasoningEffort: "high",
	}
	events := responses.Stream(t.Context(), server.Client(), cfg, []llm.Message{{
		Role:    llm.RoleUser,
		Content: "inspect",
	}}, nil, "")

	var deltas []llm.StreamDelta
	var done llm.Response
	for event, err := range events {
		require.NoError(t, err)
		switch event.Type {
		case llm.StreamEventTypeDelta:
			deltas = append(deltas, event.Delta)
		case llm.StreamEventTypeDone:
			done = event.Partial
		}
	}

	reasoning := request["reasoning"].(map[string]any)
	require.Equal(t, "high", reasoning["effort"])
	require.False(t, request["store"].(bool))
	require.Equal(t, "test", request["model"])
	require.Len(t, request["input"], 1)
	require.Equal(t, "hello", deltas[0].Content)
	require.Equal(t, "call_1", deltas[1].ToolCalls[0].ID)
	require.Equal(t, "read", deltas[1].ToolCalls[0].Function.Name)
	require.Equal(t, 11, done.Usage.PromptTokens)
	require.Equal(t, 3, done.Usage.CachedTokens())
	require.Equal(t, 5, done.Usage.CompletionTokens)
	require.Equal(t, "hello", done.Choices[0].Message.Content)
	require.Equal(t, "call_1", done.Choices[0].Message.ToolCalls[0].ID)
}
