package session

import (
	"strings"
	"time"
)

// Role is the speaker of a transcript message.
type Role int

// Role values for transcript messages.
const (
	RoleUser Role = iota
	RoleAssistant
	RoleCompaction // transcript marker with the context-compaction report
	RoleLocalBash  // user-initiated "!cmd" shell run (UI-only, not agent)
	RoleWatch      // a background watch that fired (UI-only, not agent)
	RolePlan       // a plan automation that ran (UI-only, not agent)
)

// State is the assistant message lifecycle.
type State int

// State lifecycle values.
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

// StopReason values for completed assistant messages.
const (
	StopNone StopReason = iota
	StopEndTurn
	StopToolUse
	StopMaxTokens
)

// BlockType is an assistant content block discriminant.
type BlockType int

// BlockType values for assistant content blocks.
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

// ToolStatus values for tool runs.
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
	// Queued marks a user message accepted while a run was in flight: it is
	// shown as waiting behind the running turn, not as sent.
	Queued bool
	// Summary is the compaction summarize body (RoleCompaction only); the
	// transcript row expands to show it. Empty on every other role.
	Summary string
	Content []ContentBlock
	// Usage is token consumption for the latest assistant turn (UI + diagnostics).
	// Zero means unknown / not yet reported by the provider.
	Usage TokenUsage
	// Model is the model id that produced this assistant round ("" = unknown).
	Model string
	// Started/Ended are the round's wall-clock span for the turn metadata row.
	// Ended stays zero while streaming; both zero means timing unknown
	// (e.g. replayed history).
	Started time.Time
	Ended   time.Time
	// ThinkingDuration is the wall-clock span of the round's reasoning,
	// 0 when unknown (streaming, replayed history, or no reasoning).
	ThinkingDuration time.Duration
}

// TurnDuration returns the round span when both ends are known, else 0.
func (m Message) TurnDuration() time.Duration {
	if m.Started.IsZero() || m.Ended.IsZero() {
		return 0
	}
	return m.Ended.Sub(m.Started)
}

// TokenUsage is a UI-facing copy of provider token counts for one completion.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	CachedTokens     int // prompt cache reads (c in the composer)
	TotalTokens      int
	// Estimated distinguishes post-compaction size estimates from provider
	// counters so the UI never presents an approximation as exact usage.
	Estimated bool
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
			text.WriteString(blk.Text)
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

// UserAppend appends a user message. Queued marks a submit accepted behind a
// running turn.
type UserAppend struct {
	ID     string
	Text   string
	Queued bool
}

func (UserAppend) isSessionEvent() {}

// UserPromoted clears the queued flag on the matching user row once the
// in-flight turn finishes and that prompt dequeues to run.
type UserPromoted struct {
	ID string
}

func (UserPromoted) isSessionEvent() {}

// LocalBashStart appends a user-initiated "!cmd" bash row.
type LocalBashStart struct {
	ID      string
	Command string
}

func (LocalBashStart) isSessionEvent() {}

// WatchFired appends the row for one background watch event. The row is
// UI-only: what the model is told about the event travels as an injected
// prompt, not as this transcript entry, so the two can say different things
// without either being a lie.
type WatchFired struct {
	ID    string
	Label string
	Text  string
}

func (WatchFired) isSessionEvent() {}

// AssistantMessageUpdate replaces the in-flight streaming assistant turn with
// the same turn (wherever it sits — a queued user message may have been
// appended below it), or the last assistant with the same ID, or appends a new
// assistant turn otherwise — mirrors assistant message-update semantics.
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
// appends a transcript marker backed by the durable compaction record.
type CompactionComplete struct {
	ID         string
	Failed     bool
	Compaction Compaction
}

func (CompactionComplete) isSessionEvent() {}

// PlanActionRan reports one executed plan action: which built-in ran, where
// it lives (empty StepID = the plan-level list), and its terminal outcome.
// The durable run record rides inside the plan snapshot; this event is the
// transcript row and the live status line.
type PlanActionRan struct {
	StepID string
	Event  PlanActionEvent
	Type   PlanActionType
	Status PlanActionRunStatus
	Error  string
}

func (PlanActionRan) isSessionEvent() {}

// Snapshot is the full session state the TUI projects from.
type Snapshot struct {
	Messages   []Message
	Tools      map[string]ToolRun
	Compacting bool
}
