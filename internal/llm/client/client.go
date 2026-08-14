package client

import (
	"context"
	"iter"
	"net/http"
	"strings"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/llm/anthropic"
	"github.com/pulseaiclub/phi/internal/llm/openai"
	"github.com/pulseaiclub/phi/internal/util"
)

// Client talks to the configured LLM endpoint: the OpenAI-compatible
// /chat/completions API by default, or the Anthropic Messages API when the
// config targets anthropic (see isAnthropicProvider).
type Client struct {
	httpClient *http.Client
	cfg        llm.ModelConfig
	tools      []llm.ToolDefinition
	system     string
	anthropic  bool
}

// NewClient builds a streaming chat client.
func NewClient(cfg llm.ModelConfig, tools []llm.ToolDefinition, systemPrompt string) *Client {
	return &Client{
		httpClient: util.DefaultHTTPClient(),
		cfg:        cfg,
		tools:      tools,
		system:     systemPrompt,
		anthropic:  isAnthropicProvider(cfg),
	}
}

// Stream runs a streaming chat completion over messages (+ optional system prompt / tools).
func (c *Client) Stream(ctx context.Context, messages []llm.Message) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		if c.anthropic {
			req := anthropic.BuildRequest(c.cfg, c.system, messages, c.tools)
			for ev, err := range anthropic.Stream(ctx, c.httpClient, c.cfg, &req) {
				if !yield(ev, err) {
					return
				}
			}
			return
		}
		req := openai.BuildRequest(c.cfg, c.system, messages, c.tools)
		for ev, err := range openai.StreamChatCompletion(ctx, c.httpClient, c.cfg.BaseURL, c.cfg.APIKey, req) {
			if !yield(ev, err) {
				return
			}
		}
	}
}

// Compact sends a single non-streaming chat request and returns the
// assistant text. It satisfies llm.Compactor for session compaction.
func (c *Client) Compact(ctx context.Context, prompt string) (string, error) {
	if c.anthropic {
		return anthropic.Compact(ctx, c.httpClient, c.cfg, prompt)
	}
	return openai.Compact(ctx, c.httpClient, c.cfg, prompt)
}

// isAnthropicProvider reports whether the config targets the Anthropic
// Messages API: either an anthropic base URL or a claude model name.
func isAnthropicProvider(cfg llm.ModelConfig) bool {
	base := strings.ToLower(cfg.BaseURL)
	if strings.Contains(base, "anthropic") {
		return true
	}
	return strings.HasPrefix(strings.ToLower(cfg.Name), "claude")
}
