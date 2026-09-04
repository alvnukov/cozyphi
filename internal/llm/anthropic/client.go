package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"iter"
	"net/http"
	"regexp"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/util"
)

const (
	defaultBaseURL = "https://api.anthropic.com/v1"
	apiVersion     = "2023-06-01"
	messagesPath   = "/messages"
	// fallbackMaxTokens is sent when the model config sets no explicit
	// output budget: the Messages API requires max_tokens, so "no limit
	// configured" still needs a provider-safe number every Anthropic-shaped
	// endpoint accepts.
	fallbackMaxTokens = 8192
)

// resolveMaxTokens returns the request's max_tokens: an explicit
// models[].max_output_tokens wins, otherwise the provider-safe fallback.
func resolveMaxTokens(cfg llm.ModelConfig) int {
	if cfg.MaxOutputTokens > 0 {
		return cfg.MaxOutputTokens
	}
	return fallbackMaxTokens
}

var toolCallIDRegex = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func normalizeBaseURL(baseURL string) string {
	if baseURL == "" {
		return defaultBaseURL
	}
	normalized := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(normalized, "/v1") {
		return normalized
	}
	return normalized + "/v1"
}

// BuildRequest converts the normalized messages into the Anthropic Messages
// API shape. System text and tool results are merged the same way panda does
// (consecutive tool messages become one user message with tool_result blocks;
// prompt caching is pinned to the tail of the request).
func BuildRequest(
	cfg llm.ModelConfig,
	system string,
	messages []llm.Message,
	tools []llm.ToolDefinition,
) anthropicRequest {
	cc := resolveCacheControl()

	req := anthropicRequest{
		Model:     cfg.RequestModel(),
		MaxTokens: resolveMaxTokens(cfg),
		Stream:    true,
	}

	var systemText strings.Builder
	if strings.TrimSpace(system) != "" {
		systemText.WriteString(system)
	}
	var msgs []llm.Message
	for _, m := range messages {
		if m.Role == llm.RoleSystem {
			if systemText.Len() > 0 {
				systemText.WriteByte('\n')
			}
			systemText.WriteString(m.Content)
			continue
		}
		msgs = append(msgs, m)
	}
	if systemText.Len() > 0 {
		req.System = []sysBlock{{
			Type:         "text",
			Text:         systemText.String(),
			CacheControl: cc,
		}}
	}

	for i := 0; i < len(msgs); i++ {
		m := msgs[i]
		switch m.Role {
		case llm.RoleUser:
			var content any = m.Content
			if len(m.Media) > 0 {
				blocks := make([]anthropicContentBlock, 0, 1+len(m.Media))
				if m.Content != "" {
					blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Content})
				}
				for _, media := range m.Media {
					blocks = append(blocks, anthropicContentBlock{Type: "image", Source: &imageSource{
						Type:      "base64",
						MediaType: media.MediaType,
						Data:      media.Data,
					}})
				}
				content = blocks
			}
			req.Messages = append(req.Messages, anthropicMessage{
				Role:    "user",
				Content: content,
			})

		case llm.RoleAssistant:
			msg := anthropicMessage{Role: "assistant"}
			// A thinking-mode model must get a prior turn's reasoning passed back
			// as a thinking block; dropping it triggers the API error "The
			// content[].thinking in the thinking mode must be passed back to the
			// API". Non-thinking models keep it stripped, as they reject stray
			// thinking blocks.
			thinking := ""
			if llm.IsThinkingModel(cfg.RequestModel()) {
				thinking = m.ReasoningContent
			}
			if thinking != "" || len(m.ToolCalls) > 0 {
				blocks := make([]anthropicContentBlock, 0, 1+len(m.ToolCalls))
				if thinking != "" {
					blocks = append(blocks, anthropicContentBlock{Type: "thinking", Thinking: thinking})
				}
				if m.Content != "" {
					blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Content})
				}
				for _, tc := range m.ToolCalls {
					blocks = append(blocks, anthropicContentBlock{
						Type:  "tool_use",
						ID:    normalizeToolCallID(tc.ID),
						Name:  tc.Function.Name,
						Input: toolUseInput(tc.Function.Arguments),
					})
				}
				msg.Content = blocks
			} else {
				msg.Content = m.Content
			}
			req.Messages = append(req.Messages, msg)

		case llm.RoleTool:
			blocks := make([]anthropicContentBlock, 0, 1)
			for i < len(msgs) && msgs[i].Role == llm.RoleTool {
				tm := msgs[i]
				blocks = append(blocks, anthropicContentBlock{
					Type:      "tool_result",
					ToolUseID: normalizeToolCallID(tm.ToolCallID),
					Content:   tm.Content,
				})
				i++
			}
			// Anthropic requires tool_result blocks first in the immediate
			// user message. A repaired legacy history may already contain a
			// later user prompt, so fold it in after the results rather than
			// emitting a second consecutive user message.
			for i < len(msgs) && msgs[i].Role == llm.RoleUser {
				if msgs[i].Content != "" {
					blocks = append(blocks, anthropicContentBlock{Type: "text", Text: msgs[i].Content})
				}
				i++
			}
			i--
			req.Messages = append(req.Messages, anthropicMessage{
				Role:    "user",
				Content: blocks,
			})
		}
	}

	// Pin prompt caching to the tail of the last user message, mirroring
	// panda / go-ai behavior.
	if len(req.Messages) > 0 {
		last := &req.Messages[len(req.Messages)-1]
		if last.Role == "user" {
			if blocks, ok := last.Content.([]anthropicContentBlock); ok && len(blocks) > 0 {
				blocks[len(blocks)-1].CacheControl = cc
				last.Content = blocks
			} else if text, ok := last.Content.(string); ok {
				last.Content = []anthropicContentBlock{
					{Type: "text", Text: text, CacheControl: cc},
				}
			}
		}
	}

	for i, t := range tools {
		params, _ := json.Marshal(t.Params)
		if len(params) == 0 || bytes.Equal(bytes.TrimSpace(params), []byte("null")) {
			params = json.RawMessage("{}")
		}
		tool := anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: params,
		}
		if i == len(tools)-1 {
			tool.CacheControl = cc
		}
		req.Tools = append(req.Tools, tool)
	}

	applyModelOptions(&req, cfg)
	return req
}

// applyModelOptions copies the model's effective sampling options onto a
// request body. The Messages API shares only temperature and top_p with the
// openai wire format; reasoning effort and the provider-specific thinking
// switches have no anthropic field, so they are not forwarded — a documented
// deviation for opencode imports. Both BuildRequest and Compact route through
// here so a compaction request carries the same tuning as the conversation it
// summarizes.
func applyModelOptions(req *anthropicRequest, cfg llm.ModelConfig) {
	opts := cfg.EffectiveOptions()
	req.Temperature = opts.Temperature
	req.TopP = opts.TopP
}

func normalizeToolCallID(id string) string {
	normalized := toolCallIDRegex.ReplaceAllString(id, "_")
	if len(normalized) > 64 {
		normalized = normalized[:64]
	}
	return normalized
}

func toolUseInput(arguments string) json.RawMessage {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return json.RawMessage("{}")
	}
	if json.Valid([]byte(arguments)) {
		return json.RawMessage(arguments)
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return json.RawMessage("{}")
	}
	return encoded
}

// Stream POSTs a streaming request to the Messages API and yields normalized
// events (same StreamEvent contract as the OpenAI-compatible path).
func Stream(
	ctx context.Context,
	httpClient *http.Client,
	cfg llm.ModelConfig,
	req *anthropicRequest,
) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		body, err := json.Marshal(req)
		if err != nil {
			yield(llm.StreamEvent{}, err)
			return
		}

		httpReq, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			normalizeBaseURL(cfg.BaseURL)+messagesPath,
			bytes.NewReader(body),
		)
		if err != nil {
			yield(llm.StreamEvent{}, err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-Api-Key", cfg.APIKey)
		httpReq.Header.Set("Anthropic-Version", apiVersion)
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
				llm.APIError("anthropic API error", httpResp.StatusCode, llm.ReadErrorBody(httpResp.Body)),
			)
			return
		}

		processStream(httpResp.Body, yield)
	}
}

func processStream(body io.Reader, yield func(llm.StreamEvent, error) bool) {
	var (
		content     strings.Builder
		reasoning   strings.Builder
		usage       llm.Usage
		toolCalls   []llm.ToolCall
		currentTool *llm.ToolCall
		toolArgs    strings.Builder
		stopReason  string
	)

	for data, parseErr := range util.ParseDataStream(body) {
		if parseErr != nil {
			yield(llm.StreamEvent{Type: llm.StreamEventTypeError, Err: parseErr}, parseErr)
			return
		}
		payloadLine := bytes.TrimSpace(data)
		if len(payloadLine) == 0 {
			continue
		}

		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(payloadLine, &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "message_start":
			var msg struct {
				Message struct {
					Usage struct {
						InputTokens int `json:"input_tokens"`
						CacheRead   int `json:"cache_read_input_tokens"`
						CacheCreate int `json:"cache_creation_input_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if err := json.Unmarshal(payloadLine, &msg); err != nil {
				continue
			}
			u := msg.Message.Usage
			// Anthropic splits input into disjoint fields; OpenAI-shaped
			// PromptTokens is the full prompt occupancy (cache is a subset).
			usage.PromptTokens = u.InputTokens + u.CacheRead + u.CacheCreate
			if u.CacheRead > 0 {
				usage.PromptTokensDetails = &llm.PromptTokensDetails{
					CachedTokens: u.CacheRead,
				}
			}

		case "content_block_start":
			var block struct {
				Index        int `json:"index"`
				ContentBlock struct {
					Type string `json:"type"`
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"content_block"`
			}
			if err := json.Unmarshal(payloadLine, &block); err != nil {
				continue
			}
			if block.ContentBlock.Type == "tool_use" {
				currentTool = &llm.ToolCall{
					Index: block.Index,
					ID:    block.ContentBlock.ID,
					Type:  "function",
					Function: llm.Function{
						Name: block.ContentBlock.Name,
					},
				}
			}

		case "content_block_delta":
			var block struct {
				Index int `json:"index"`
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					Thinking    string `json:"thinking"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			}
			if err := json.Unmarshal(payloadLine, &block); err != nil {
				continue
			}

			switch block.Delta.Type {
			case "text_delta":
				content.WriteString(block.Delta.Text)
				if !yield(llm.StreamEvent{
					Type:    llm.StreamEventTypeDelta,
					Delta:   llm.StreamDelta{Content: block.Delta.Text},
					Partial: llm.Response{Usage: usage},
				}, nil) {
					return
				}

			case "thinking_delta":
				reasoning.WriteString(block.Delta.Thinking)
				if !yield(llm.StreamEvent{
					Type:    llm.StreamEventTypeDelta,
					Delta:   llm.StreamDelta{ReasoningContent: block.Delta.Thinking},
					Partial: llm.Response{Usage: usage},
				}, nil) {
					return
				}

			case "input_json_delta":
				if currentTool == nil {
					continue
				}
				toolArgs.WriteString(block.Delta.PartialJSON)
				currentTool.Function.Arguments = toolArgs.String()
				if !yield(llm.StreamEvent{
					Type: llm.StreamEventTypeDelta,
					Delta: llm.StreamDelta{ToolCalls: []llm.ToolCall{
						{
							Index: currentTool.Index,
							ID:    currentTool.ID,
							Type:  currentTool.Type,
							Function: llm.Function{
								Name:      currentTool.Function.Name,
								Arguments: currentTool.Function.Arguments,
							},
						},
					}},
					Partial: llm.Response{Usage: usage},
				}, nil) {
					return
				}
			}

		case "content_block_stop":
			var block struct {
				Index int `json:"index"`
			}
			if err := json.Unmarshal(payloadLine, &block); err != nil {
				continue
			}
			if currentTool != nil && block.Index == currentTool.Index {
				currentTool.Function.Arguments = toolArgs.String()
				toolCalls = append(toolCalls, *currentTool)
				currentTool = nil
				toolArgs.Reset()
			}

		case "message_delta":
			var msgDelta struct {
				Delta struct {
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
				Usage struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal(payloadLine, &msgDelta); err != nil {
				continue
			}
			stopReason = msgDelta.Delta.StopReason
			usage.CompletionTokens = msgDelta.Usage.OutputTokens
		}
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	out := llm.Response{
		Choices: []llm.Choice{{
			FinishReason: stopReason,
			Message: llm.Message{
				Role:             llm.RoleAssistant,
				Content:          content.String(),
				ReasoningContent: reasoning.String(),
				ToolCalls:        toolCalls,
			},
		}},
		Usage: usage,
	}
	yield(llm.StreamEvent{Type: llm.StreamEventTypeDone, Partial: out}, nil)
}

// Compact sends a single non-streaming request and returns the assistant
// text. Satisfies llm.Compactor for session compaction on Claude.
func Compact(ctx context.Context, httpClient *http.Client, cfg llm.ModelConfig, prompt string) (string, error) {
	req := anthropicRequest{
		Model:     cfg.RequestModel(),
		MaxTokens: resolveMaxTokens(cfg),
		Messages: []anthropicMessage{
			{Role: "user", Content: prompt},
		},
	}
	applyModelOptions(&req, cfg)
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		normalizeBaseURL(cfg.BaseURL)+messagesPath,
		bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Api-Key", cfg.APIKey)
	httpReq.Header.Set("Anthropic-Version", apiVersion)

	httpResp, err := util.DoWithRetry(httpClient, httpReq)
	if err != nil {
		return "", err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return "", llm.APIError("anthropic API error", httpResp.StatusCode, llm.ReadErrorBody(httpResp.Body))
	}
	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, llm.MaxResponseBytes))
	if err != nil {
		return "", err
	}

	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}
	if sb.Len() == 0 {
		return "", errors.New("anthropic API error: empty response")
	}
	return sb.String(), nil
}
