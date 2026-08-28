package client

import (
	"context"
	"iter"
	"net/http"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/llm/anthropic"
	"github.com/alvnukov/cozyphi/internal/llm/openai"
	"github.com/alvnukov/cozyphi/internal/llm/responses"
	"github.com/alvnukov/cozyphi/internal/util"
)

// Client talks to the configured LLM endpoint: the OpenAI-compatible
// /chat/completions API by default, or the Anthropic Messages API when the
// config's protocol targets anthropic.
type Client struct {
	httpClient *http.Client
	cfg        llm.ModelConfig
	tools      []llm.ToolDefinition
	system     string
}

// NewClient builds a streaming chat client.
func NewClient(cfg llm.ModelConfig, tools []llm.ToolDefinition, systemPrompt string) *Client {
	return &Client{
		httpClient: util.DefaultHTTPClient(),
		cfg:        cfg,
		tools:      tools,
		system:     systemPrompt,
	}
}

// Stream runs a streaming chat completion over messages (+ optional system prompt / tools).
func (c *Client) Stream(ctx context.Context, messages []llm.Message) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		messages, _ = llm.RepairToolHistory(messages)
		switch c.cfg.Protocol {
		case llm.ProtocolAnthropic:
			req := anthropic.BuildRequest(c.cfg, c.system, messages, c.tools)
			for ev, err := range anthropic.Stream(ctx, c.httpClient, c.cfg, &req) {
				if !yield(ev, err) {
					return
				}
			}
		case llm.ProtocolOpenAIResponses:
			for ev, err := range responses.Stream(ctx, c.httpClient, c.cfg, messages, c.tools, c.system) {
				if !yield(ev, err) {
					return
				}
			}
		default:
			req := openai.BuildRequest(c.cfg, c.system, messages, c.tools)
			for ev, err := range openai.StreamChatCompletion(ctx, c.httpClient, c.cfg.BaseURL, c.cfg.APIKey, req) {
				if !yield(ev, err) {
					return
				}
			}
		}
	}
}

// Compact sends a single non-streaming chat request and returns the
// assistant text. It satisfies llm.Compactor for session compaction.
func (c *Client) Compact(ctx context.Context, prompt string) (string, error) {
	switch c.cfg.Protocol {
	case llm.ProtocolAnthropic:
		return anthropic.Compact(ctx, c.httpClient, c.cfg, prompt)
	case llm.ProtocolOpenAIResponses:
		return responses.Compact(ctx, c.httpClient, c.cfg, prompt)
	default:
		return openai.Compact(ctx, c.httpClient, c.cfg, prompt)
	}
}
