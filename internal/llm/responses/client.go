package responses

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/util"
)

const (
	responsesPath  = "/responses"
	maxEventBytes  = 10 * 1024 * 1024
	initialSSESize = 64 * 1024
)

type apiRequest struct {
	Model           string           `json:"model"`
	Instructions    string           `json:"instructions,omitempty"`
	Input           []any            `json:"input"`
	Tools           []apiTool        `json:"tools,omitempty"`
	Store           bool             `json:"store"`
	Stream          bool             `json:"stream"`
	MaxOutputTokens int              `json:"max_output_tokens,omitempty"`
	Reasoning       *reasoningConfig `json:"reasoning,omitempty"`
	Include         []string         `json:"include,omitempty"`
}

type reasoningConfig struct {
	Effort string `json:"effort"`
}

type apiTool struct {
	Type        string                  `json:"type"`
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Parameters  *llm.FunctionParameters `json:"parameters"`
}

type responseEvent struct {
	Type     string          `json:"type"`
	Delta    string          `json:"delta,omitempty"`
	Item     json.RawMessage `json:"item,omitempty"`
	Response responseBody    `json:"response,omitempty"`
	Message  string          `json:"message,omitempty"`
	Code     string          `json:"code,omitempty"`
}

type responseBody struct {
	Status string         `json:"status"`
	Error  *responseError `json:"error,omitempty"`
	Usage  responseUsage  `json:"usage"`
}

type responseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type responseUsage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	TotalTokens        int `json:"total_tokens"`
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
}

type outputItem struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type accumulator struct {
	content       strings.Builder
	reasoning     strings.Builder
	toolCalls     []llm.ToolCall
	providerState []json.RawMessage
	usage         llm.Usage
}

// Stream sends a stateless OpenAI Responses request and normalizes its SSE
// events. Provider output items are retained so encrypted reasoning and tool
// calls survive the next turn without enabling server-side response storage.
func Stream(
	ctx context.Context,
	httpClient *http.Client,
	cfg llm.ModelConfig,
	messages []llm.Message,
	tools []llm.ToolDefinition,
	systemPrompt string,
) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		reqBody, err := buildRequest(cfg, messages, tools, systemPrompt)
		if err != nil {
			yield(llm.StreamEvent{}, err)
			return
		}
		body, err := json.Marshal(reqBody)
		if err != nil {
			yield(llm.StreamEvent{}, fmt.Errorf("encode Responses request: %w", err))
			return
		}
		url := strings.TrimRight(cfg.BaseURL, "/") + responsesPath
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			yield(llm.StreamEvent{}, fmt.Errorf("create Responses request: %w", err))
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		if cfg.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		}
		if cfg.Authenticator != nil {
			if err := cfg.Authenticator.Authorize(ctx, req); err != nil {
				yield(llm.StreamEvent{}, fmt.Errorf("authorize Responses request: %w", err))
				return
			}
		}

		resp, err := util.DoWithRetry(httpClient, req)
		if err != nil {
			yield(llm.StreamEvent{}, fmt.Errorf("send Responses request: %w", err))
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			// Classified from the body, never echoing it: a hostile or
			// misconfigured endpoint can reflect request material back.
			body := llm.ReadErrorBody(resp.Body)
			err := fmt.Errorf("responses API error: status %d", resp.StatusCode)
			if resp.StatusCode == http.StatusRequestEntityTooLarge {
				err = fmt.Errorf("%w: %w", llm.ErrContextOverflow, err)
			} else {
				err = llm.MarkContextOverflow(err, string(body))
			}
			// Status rides on the wrap chain: downstream code branches on
			// the code (llm.IsRateLimited / IsAuthFailure), not on text.
			yield(llm.StreamEvent{}, &llm.StatusError{Status: resp.StatusCode, Cause: err})
			return
		}

		acc := &accumulator{}
		completed, stopped, err := readEvents(resp.Body, func(event responseEvent) bool {
			return handleEvent(event, acc, yield)
		})
		if stopped {
			return
		}
		if err != nil {
			yield(llm.StreamEvent{}, err)
			return
		}
		if !completed {
			yield(llm.StreamEvent{}, errors.New("responses stream ended before response.completed"))
		}
	}
}

func buildRequest(
	cfg llm.ModelConfig,
	messages []llm.Message,
	tools []llm.ToolDefinition,
	systemPrompt string,
) (apiRequest, error) {
	input := make([]any, 0, len(messages))
	for _, message := range messages {
		if len(message.ProviderState) > 0 {
			for _, raw := range message.ProviderState {
				if !json.Valid(raw) {
					return apiRequest{}, errors.New("invalid persisted Responses provider state")
				}
				input = append(input, raw)
			}
			continue
		}
		switch message.Role {
		case llm.RoleTool:
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": message.ToolCallID,
				"output":  message.Content,
			})
		case llm.RoleAssistant:
			if message.Content != "" {
				input = append(input, map[string]any{
					"type": "message", "role": "assistant", "content": message.Content,
				})
			}
			for _, call := range message.ToolCalls {
				input = append(input, map[string]any{
					"type": "function_call", "call_id": call.ID,
					"name": call.Function.Name, "arguments": call.Function.Arguments,
				})
			}
		default:
			input = append(input, map[string]any{
				"type": "message", "role": string(message.Role), "content": responsesContent(message),
			})
		}
	}
	apiTools := make([]apiTool, 0, len(tools))
	for _, tool := range tools {
		apiTools = append(apiTools, apiTool{
			Type: "function", Name: tool.Name, Description: tool.Description, Parameters: tool.Params,
		})
	}
	req := apiRequest{
		Model: cfg.RequestModel(), Instructions: systemPrompt, Input: input,
		Tools: apiTools, Store: false, Stream: true,
		MaxOutputTokens: cfg.MaxOutputTokens,
		Include:         []string{"reasoning.encrypted_content"},
	}
	if cfg.ReasoningEffort != "" {
		effort, ok := llm.ParseReasoningEffort(string(cfg.ReasoningEffort))
		if !ok {
			return apiRequest{}, fmt.Errorf("unsupported reasoning effort %q", cfg.ReasoningEffort)
		}
		req.Reasoning = &reasoningConfig{Effort: string(effort)}
	}
	return req, nil
}

// responsesContent renders a user message as a string, or as content parts when
// it carries inline media (images). The text part comes first so the model reads
// the instruction before the picture.
func responsesContent(message llm.Message) any {
	if len(message.Media) == 0 {
		return message.Content
	}
	parts := make([]any, 0, 1+len(message.Media))
	if message.Content != "" {
		parts = append(parts, map[string]any{"type": "input_text", "text": message.Content})
	}
	for _, media := range message.Media {
		parts = append(parts, map[string]any{
			"type":      "input_image",
			"image_url": "data:" + media.MediaType + ";base64," + media.Data,
		})
	}
	return parts
}

func readEvents(body io.Reader, handle func(responseEvent) bool) (completed, stopped bool, err error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, initialSSESize), maxEventBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(line[len("data:"):])
		if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
			continue
		}
		var event responseEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return false, false, fmt.Errorf("decode Responses event: %w", err)
		}
		if event.Type == "response.completed" {
			completed = true
		}
		if !handle(event) {
			return completed, true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, false, fmt.Errorf("read Responses stream: %w", err)
	}
	return completed, false, nil
}

func handleEvent(
	event responseEvent,
	acc *accumulator,
	yield func(llm.StreamEvent, error) bool,
) bool {
	switch event.Type {
	case "response.output_text.delta":
		acc.content.WriteString(event.Delta)
		return yield(llm.StreamEvent{
			Type:  llm.StreamEventTypeDelta,
			Delta: llm.StreamDelta{Content: event.Delta},
		}, nil)
	case "response.reasoning_summary_text.delta":
		acc.reasoning.WriteString(event.Delta)
		return yield(llm.StreamEvent{
			Type:  llm.StreamEventTypeDelta,
			Delta: llm.StreamDelta{ReasoningContent: event.Delta},
		}, nil)
	case "response.output_item.done":
		if len(event.Item) == 0 {
			return true
		}
		acc.providerState = append(acc.providerState, append(json.RawMessage(nil), event.Item...))
		var item outputItem
		if err := json.Unmarshal(event.Item, &item); err != nil || item.Type != "function_call" {
			return true
		}
		call := llm.ToolCall{
			Index: len(acc.toolCalls), ID: item.CallID, Type: "function",
			Function: llm.Function{Name: item.Name, Arguments: item.Arguments},
		}
		acc.toolCalls = append(acc.toolCalls, call)
		return yield(llm.StreamEvent{
			Type:  llm.StreamEventTypeDelta,
			Delta: llm.StreamDelta{ToolCalls: []llm.ToolCall{call}},
		}, nil)
	case "response.completed":
		acc.usage = normalizeUsage(event.Response.Usage)
		finish := "stop"
		if len(acc.toolCalls) > 0 {
			finish = "tool_calls"
		}
		message := llm.Message{
			Role: llm.RoleAssistant, Content: acc.content.String(),
			ReasoningContent: acc.reasoning.String(), ToolCalls: acc.toolCalls,
			ProviderState: acc.providerState,
		}
		return yield(llm.StreamEvent{
			Type: llm.StreamEventTypeDone,
			Partial: llm.Response{
				Choices: []llm.Choice{{Message: message, FinishReason: finish}},
				Usage:   acc.usage,
			},
		}, nil)
	case "error":
		message := strings.TrimSpace(event.Message)
		if message == "" {
			message = "unknown stream error"
		}
		return yield(
			llm.StreamEvent{},
			llm.MarkContextOverflow(fmt.Errorf("responses stream error (%s): %s", event.Code, message), message),
		)
	case "response.failed", "response.incomplete":
		message := "request did not complete"
		if event.Response.Error != nil && event.Response.Error.Message != "" {
			message = event.Response.Error.Message
		}
		return yield(
			llm.StreamEvent{},
			llm.MarkContextOverflow(fmt.Errorf("responses %s: %s", event.Response.Status, message), message),
		)
	default:
		return true
	}
}

// Compact runs the same protocol without tools and returns only the completed
// assistant text. An incomplete stream is an error, never an empty summary.
func Compact(ctx context.Context, httpClient *http.Client, cfg llm.ModelConfig, prompt string) (string, error) {
	var content string
	for event, err := range Stream(ctx, httpClient, cfg, []llm.Message{{
		Role: llm.RoleUser, Content: prompt,
	}}, nil, "") {
		if err != nil {
			return "", err
		}
		if event.Type == llm.StreamEventTypeDone && len(event.Partial.Choices) > 0 {
			content = event.Partial.Choices[0].Message.Content
		}
	}
	if strings.TrimSpace(content) == "" {
		return "", errors.New("responses compaction returned no text")
	}
	return content, nil
}

func normalizeUsage(usage responseUsage) llm.Usage {
	return llm.Usage{
		PromptTokens:        usage.InputTokens,
		CompletionTokens:    usage.OutputTokens,
		TotalTokens:         usage.TotalTokens,
		PromptTokensDetails: &llm.PromptTokensDetails{CachedTokens: usage.InputTokensDetails.CachedTokens},
	}
}
