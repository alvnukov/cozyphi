package session

import "strings"

// Role is the speaker of a transcript message.
type Role int

const (
	RoleUser Role = iota
	RoleAssistant
	RoleCompaction // transcript marker after context compaction ("Compacted")
	RoleLocalBash  // user-initiated "!cmd" shell run (UI-only, not agent)
)

// State is the assistant message lifecycle.
type State int

const (
	StateStreaming State = iota
	StateComplete
	StateCancelled
	StateError
)

func (s State) String() string {
	switch s {
	case StateStreaming:
		return "streaming"
	case StateComplete:
		return "complete"
	case StateCancelled:
		return "cancelled"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// StopReason is set when an assistant message completes.
type StopReason int

const (
	StopNone StopReason = iota
	StopEndTurn
	StopToolUse
	StopMaxTokens
)

// BlockType is an assistant content block discriminant.
type BlockType int

const (
	BlockText BlockType = iota
	BlockThinking
	BlockToolUse
)

// ContentBlock is one assistant content part.
type ContentBlock struct {
	Type BlockType

	// Text / Thinking
	Text string

	// ToolUse
	ID       string
	Name     string
	Input    string // display / JSON-ish input
	Complete bool
}

// ToolStatus is the tool run status.
type ToolStatus int

const (
	ToolQueued ToolStatus = iota
	ToolInProgress
	ToolDone
	ToolError
	ToolCancelled
	ToolRejected
)

func (s ToolStatus) String() string {
	switch s {
	case ToolQueued:
		return "queued"
	case ToolInProgress:
		return "in-progress"
	case ToolDone:
		return "done"
	case ToolError:
		return "error"
	case ToolCancelled:
		return "cancelled"
	case ToolRejected:
		return "rejected"
	default:
		return "unknown"
	}
}

// ToolRun is the live execution state for a tool_use id.
type ToolRun struct {
	ToolUseID string
	Name      string // tool name (bash, read, ...)
	Status    ToolStatus
	Output    string
	Error     string
	Detail    string // optional one-line detail (path, cmd summary)
	ExitCode  int    // set when a local bash run finishes (Status Done/Error)
	Local     bool   // user "!cmd" bash; ignored by agent streaming/busy checks
}

// Message is one session message. Assistant rows carry Content blocks and State.
type Message struct {
	ID         string
	Role       Role
	State      State      // assistant
	StopReason StopReason // assistant when complete
	Text       string     // user visible text
	Content    []ContentBlock
	// Usage is token consumption for the latest assistant turn (UI + diagnostics).
	// Zero means unknown / not yet reported by the provider.
	Usage TokenUsage
}

// TokenUsage is a UI-facing copy of provider token counts for one completion.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	CachedTokens     int // prompt cache reads (⚡ in the composer)
	TotalTokens      int
}

// Reported is true when the provider sent any non-zero token count.
func (u TokenUsage) Reported() bool {
	return u.TotalTokens > 0 || u.PromptTokens > 0 || u.CompletionTokens > 0 || u.CachedTokens > 0
}

// ContextTokens is the best available estimate of tokens occupying the context
// window (prefer prompt/input; fall back to total).
func (u TokenUsage) ContextTokens() int {
	if u.PromptTokens > 0 {
		return u.PromptTokens
	}
	return u.TotalTokens
}

// FlatText joins assistant text blocks.
func (m Message) FlatText() string {
	if m.Role == RoleUser {
		return m.Text
	}
	var text strings.Builder
	for _, blk := range m.Content {
		if blk.Type == BlockText {
			_, _ = text.WriteString(blk.Text)
		}
	}
	out := text.String()
	if out == "" {
		return m.Text
	}
	return out
}

// Event is applied to session state.
type Event interface {
	isSessionEvent()
}

// UserAppend appends a user message.
type UserAppend struct {
	ID   string
	Text string
}

func (UserAppend) isSessionEvent() {}

// LocalBashStart appends a user-initiated "!cmd" bash row.
type LocalBashStart struct {
	ID      string
	Command string
}

func (LocalBashStart) isSessionEvent() {}

// AssistantMessageUpdate replaces the last assistant with the same turn, or
// appends if the last message is not a streaming/incomplete assistant —
// mirrors assistant message-update semantics.
type AssistantMessageUpdate struct {
	Message Message
}

func (AssistantMessageUpdate) isSessionEvent() {}

// ToolData updates a tool run by tool_use id.
type ToolData struct {
	Run ToolRun
}

func (ToolData) isSessionEvent() {}

// CancelStreaming marks the current streaming assistant as cancelled and
// cancels in-progress / queued tools.
type CancelStreaming struct{}

func (CancelStreaming) isSessionEvent() {}

// CompactionStarted signals the UI that context compaction is in progress.
type CompactionStarted struct{}

func (CompactionStarted) isSessionEvent() {}

// CompactionComplete clears the compacting activity and, when Failed is false,
// appends a "Compacted" transcript marker.
type CompactionComplete struct {
	ID     string
	Failed bool
}

func (CompactionComplete) isSessionEvent() {}

// Snapshot is the full session state the TUI projects from.
type Snapshot struct {
	Messages   []Message
	Tools      map[string]ToolRun
	Compacting bool
}
