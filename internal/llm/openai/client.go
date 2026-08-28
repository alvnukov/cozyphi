package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"iter"
	"net/http"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/util"
)

const chatCompletionsPath = "/chat/completions"

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type apiTool struct {
	Type     string             `json:"type"`
	Function llm.ToolDefinition `json:"function"`
}

type apiMessage struct {
	Role             llm.Role       `json:"role"`
	Content          any            `json:"content"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        []llm.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
}

type apiRequest struct {
	Model    string       `json:"model"`
	Messages []apiMessage `json:"messages"`
	// MaxTokens is sent only when the model config sets an explicit output
	// budget; providers apply their own default otherwise.
	MaxTokens     int            `json:"max_tokens,omitempty"`
	Tools         []apiTool      `json:"tools,omitempty"`
	Stream        bool           `json:"stream,omitempty"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
	ExtraBody     *ExtraBody     `json:"extra_body,omitempty"`
	// ReasoningEffort is sent for providers that support reasoning levels
	// (e.g. GLM-5.x on Z.AI). Empty leaves the field out.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

// ExtraBody holds provider-specific request fields (e.g. DeepSeek thinking).
type ExtraBody struct {
	Thinking *ThinkingConfig `json:"thinking,omitempty"`
}

// ThinkingConfig enables reasoning mode.
type ThinkingConfig struct {
	Type string `json:"type"`
}

// StreamChunk is a raw SSE chunk from the provider.
type StreamChunk struct {
	Choices []StreamChoice `json:"choices"`
	Usage   *llm.Usage     `json:"usage,omitempty"`
}

// StreamChoice is one streaming choice.
type StreamChoice struct {
	Delta        llm.StreamDelta `json:"delta"`
	Message      *llm.Message    `json:"message,omitempty"`
	FinishReason string          `json:"finish_reason,omitempty"`
}

func toAPIMessages(messages []llm.Message) []apiMessage {
	out := make([]apiMessage, 0, len(messages))
	for _, message := range messages {
		out = append(out, apiMessage{
			Role: message.Role, Content: apiContent(message),
			ReasoningContent: message.ReasoningContent,
			ToolCalls:        message.ToolCalls, ToolCallID: message.ToolCallID,
		})
	}
	return out
}

// apiContent renders a user message as a string, or as content parts when it
// carries inline media (images). The text part comes first so the model reads
// the instruction before the picture.
func apiContent(message llm.Message) any {
	if len(message.Media) == 0 {
		return message.Content
	}
	parts := make([]any, 0, 1+len(message.Media))
	if message.Content != "" {
		parts = append(parts, map[string]any{"type": "text", "text": message.Content})
	}
	for _, media := range message.Media {
		parts = append(parts, map[string]any{
			"type":      "image_url",
			"image_url": map[string]any{"url": "data:" + media.MediaType + ";base64," + media.Data},
		})
	}
	return parts
}

// BuildRequest converts the normalized messages into an OpenAI-shaped request.
// The system prompt is prepended as a system message, mirroring the previous
// in-client behavior.
func BuildRequest(cfg llm.ModelConfig, system string, messages []llm.Message, tools []llm.ToolDefinition) *apiRequest {
	msgs := make([]apiMessage, 0, len(messages)+1)
	if strings.TrimSpace(system) != "" {
		msgs = append(msgs, apiMessage{Role: llm.RoleSystem, Content: system})
	}
	msgs = append(msgs, toAPIMessages(messages)...)

	apiTools := make([]apiTool, len(tools))
	for i, t := range tools {
		apiTools[i] = apiTool{Type: "function", Function: t}
	}

	modelName := cfg.RequestModel()
	var extra *ExtraBody
	if isThinkingModeModel(modelName) {
		extra = &ExtraBody{Thinking: &ThinkingConfig{Type: "enabled"}}
	}

	return &apiRequest{
		Model:           modelName,
		Messages:        msgs,
		MaxTokens:       cfg.MaxOutputTokens,
		Tools:           apiTools,
		Stream:          true,
		StreamOptions:   &streamOptions{IncludeUsage: true},
		ExtraBody:       extra,
		ReasoningEffort: string(cfg.ReasoningEffort),
	}
}

func isThinkingModeModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(model), "deepseek")
}

// Compact sends a single non-streaming chat request and returns the assistant
// text. Satisfies llm.Compactor for session compaction.
func Compact(ctx context.Context, httpClient *http.Client, cfg llm.ModelConfig, prompt string) (string, error) {
	body, err := json.Marshal(&apiRequest{
		Model:           cfg.RequestModel(),
		Messages:        []apiMessage{{Role: llm.RoleUser, Content: prompt}},
		ReasoningEffort: string(cfg.ReasoningEffort),
	})
	if err != nil {
		return "", err
	}

	url := cfg.BaseURL
	if !strings.HasSuffix(url, chatCompletionsPath) {
		url += chatCompletionsPath
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	httpResp, err := util.DoWithRetry(httpClient, httpReq)
	if err != nil {
		return "", err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return "", llm.APIError("LLM API error", httpResp.StatusCode, llm.ReadErrorBody(httpResp.Body))
	}
	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, llm.MaxResponseBytes))
	if err != nil {
		return "", err
	}

	var resp llm.Response
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("LLM API error: empty choices")
	}
	return resp.Choices[0].Message.Content, nil
}

// StreamChatCompletion POSTs a streaming chat completion and yields normalized events.
func StreamChatCompletion(
	ctx context.Context,
	httpClient *http.Client,
	baseURL string,
	apiKey string,
	payload any,
) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		body, err := json.Marshal(payload)
		if err != nil {
			yield(llm.StreamEvent{}, err)
			return
		}

		url := baseURL
		if !strings.HasSuffix(url, chatCompletionsPath) {
			url += chatCompletionsPath
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			yield(llm.StreamEvent{}, err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		httpReq.Header.Set("Accept", util.ContentEventStream)

		httpResp, err := util.DoWithRetry(httpClient, httpReq)
		if err != nil {
			yield(llm.StreamEvent{}, err)
			return
		}
		defer httpResp.Body.Close()

		if httpResp.StatusCode != http.StatusOK {
			yield(
				llm.StreamEvent{},
				llm.APIError("LLM API error", httpResp.StatusCode, llm.ReadErrorBody(httpResp.Body)),
			)
			return
		}

		out := llm.Response{}
		acc := newStreamAccumulator()
		var finish string

		for data, parseErr := range util.ParseDataStream(httpResp.Body) {
			if parseErr != nil {
				yield(llm.StreamEvent{}, parseErr)
				return
			}
			payloadLine := bytes.TrimSpace(data)
			if len(payloadLine) == 0 {
				continue
			}
			if bytes.Equal(payloadLine, []byte("[DONE]")) {
				break
			}
			decodeData := data
			if bytes.Contains(decodeData, []byte("\t")) {
				decodeData = bytes.ReplaceAll(decodeData, []byte("\t"), []byte(" "))
			}

			var chunk StreamChunk
			if err := json.Unmarshal(decodeData, &chunk); err != nil {
				continue
			}
			if chunk.Usage != nil {
				out.Usage = *chunk.Usage
			}
			if len(chunk.Choices) == 0 {
				continue
			}

			sc := chunk.Choices[0]
			delta := sc.Delta
			if sc.FinishReason != "" {
				finish = sc.FinishReason
			}
			acc.applyDelta(delta)
			if sc.Message != nil {
				acc.applyMessage(sc.Message)
			}

			if hasStreamDelta(delta, sc.Message) {
				if !yield(llm.StreamEvent{
					Type:    llm.StreamEventTypeDelta,
					Delta:   delta,
					Partial: llm.Response{Usage: out.Usage},
				}, nil) {
					return
				}
			}
		}

		msg := acc.message()
		out.Choices = []llm.Choice{{Message: msg, FinishReason: finish}}
		yield(llm.StreamEvent{Type: llm.StreamEventTypeDone, Partial: out}, nil)
	}
}

func hasStreamDelta(delta llm.StreamDelta, msg *llm.Message) bool {
	if delta.Content != "" || delta.ReasoningContent != "" || delta.Role != "" || len(delta.ToolCalls) > 0 {
		return true
	}
	if msg == nil {
		return false
	}
	return strings.TrimSpace(msg.Content) != "" ||
		strings.TrimSpace(msg.ReasoningContent) != "" ||
		len(msg.ToolCalls) > 0
}
