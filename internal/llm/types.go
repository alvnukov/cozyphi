package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// Compactor compresses conversation history into a concise summary.
// Implemented by *client.Client; consumed by session compaction.
type Compactor interface {
	Compact(ctx context.Context, summary string) (string, error)
}

// Protocol identifies the wire protocol used by a model endpoint.
type Protocol string

// Supported model endpoint protocols.
const (
	ProtocolOpenAI          Protocol = "openai"
	ProtocolOpenAIResponses Protocol = "openai-responses"
	ProtocolAnthropic       Protocol = "anthropic"
)

// ReasoningEffort selects provider reasoning depth.
type ReasoningEffort string

// Supported reasoning effort values.
const (
	ReasoningEffortMinimal ReasoningEffort = "minimal"
	ReasoningEffortLow     ReasoningEffort = "low"
	ReasoningEffortMedium  ReasoningEffort = "medium"
	ReasoningEffortHigh    ReasoningEffort = "high"
)

// ParseReasoningEffort trims and validates a provider reasoning effort value.
func ParseReasoningEffort(value string) (ReasoningEffort, bool) {
	switch effort := ReasoningEffort(strings.ToLower(strings.TrimSpace(value))); effort {
	case "":
		return "", true
	case ReasoningEffortMinimal, ReasoningEffortLow, ReasoningEffortMedium, ReasoningEffortHigh:
		return effort, true
	default:
		return "", false
	}
}

// RequestAuthenticator applies short-lived or provider-specific credentials
// immediately before an HTTP request is sent.
type RequestAuthenticator interface {
	Authorize(context.Context, *http.Request) error
}

// ModelConfig is the resolved connection config for one model endpoint.
// Name is the user-facing selector. APIName is the provider's wire model ID;
// when empty, RequestModel falls back to Name for legacy configurations.
type ModelConfig struct {
	Name          string
	APIName       string
	ProviderID    string
	Protocol      Protocol
	APIKey        string
	BaseURL       string
	Authenticator RequestAuthenticator
	// SkillPath is the directory to scan for SKILL.md files.
	// Defaults to ~/.cozyphi/skills if empty.
	SkillPath string
	// ContextWindow is the model's context window in tokens.
	// Zero disables session compaction (safe default).
	ContextWindow int
	// MaxOutputTokens caps the model's output tokens per round when set.
	// Zero leaves the limit to the provider (or the client's safe fallback
	// where the API demands the field).
	MaxOutputTokens int
	// ReasoningEffort selects provider reasoning depth (minimal/low/medium/high)
	// for protocols that support it, such as OpenAI Responses.
	ReasoningEffort ReasoningEffort
}

// RequestModel returns the model identifier sent over the provider protocol.
func (c ModelConfig) RequestModel() string {
	if c.APIName != "" {
		return c.APIName
	}
	return c.Name
}

// Role identifies the participant in a chat message.
type Role string

// Role values identify the participant in a chat message.
const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall is a model-requested tool invocation.
type ToolCall struct {
	Index    int      `json:"index,omitempty"`
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// Function describes the tool name and JSON arguments.
type Function struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Message is one chat turn (OpenAI-compatible shape, normalized across
// providers).
type Message struct {
	Role             Role              `json:"role"`
	Content          string            `json:"content"`
	ReasoningContent string            `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall        `json:"tool_calls,omitempty"`
	ToolCallID       string            `json:"tool_call_id,omitempty"`
	ProviderState    []json.RawMessage `json:"provider_state,omitempty"`

	// Usage tracks token consumption for the turn. Excluded from the API
	// request body; used by the session manager for compaction decisions.
	Usage Usage `json:"-"`
}

// PromptTokensDetails holds breakdown details for prompt token usage
// (OpenAI-compatible prompt_tokens_details).
type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// Usage summarizes token consumption.
type Usage struct {
	CompletionTokens    int                  `json:"completion_tokens"`
	PromptTokens        int                  `json:"prompt_tokens"`
	TotalTokens         int                  `json:"total_tokens"`
	PromptTokensDetails *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
}

// CachedTokens returns cache-read tokens when the provider reported them.
func (u Usage) CachedTokens() int {
	if u.PromptTokensDetails == nil {
		return 0
	}
	return u.PromptTokensDetails.CachedTokens
}

// Response is a completed chat completion.
type Response struct {
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice is one completion choice.
type Choice struct {
	Message Message `json:"message"`
	// FinishReason is the provider's raw stop signal ("end_turn",
	// "max_tokens", "stop", "length", "tool_use", "tool_calls"); ""
	// when the provider did not report one.
	FinishReason string `json:"finish_reason,omitempty"`
}

// StreamDelta carries incremental content.
type StreamDelta struct {
	Role             string     `json:"role,omitempty"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
}

// StreamEventType categorizes stream events.
type StreamEventType string

// StreamEventType values categorize stream events.
const (
	StreamEventTypeDelta StreamEventType = "delta"
	StreamEventTypeDone  StreamEventType = "done"
	StreamEventTypeError StreamEventType = "error"
)

// StreamEvent is yielded during streaming.
type StreamEvent struct {
	Type    StreamEventType `json:"type"`
	Delta   StreamDelta     `json:"delta,omitempty"`
	Partial Response        `json:"partial,omitempty"`
	Err     string          `json:"err,omitempty"`
}

// Object is a JSON-schema properties map.
type Object = map[string]any

// ToolDefinition describes a function tool for the model.
type ToolDefinition struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Params      *FunctionParameters `json:"parameters"`
	Readable    bool                `json:"-"`
}

// FunctionParameters is JSON Schema for tool params.
type FunctionParameters struct {
	Type       string   `json:"type"`
	Properties Object   `json:"properties"`
	Required   []string `json:"required,omitempty"`
}
