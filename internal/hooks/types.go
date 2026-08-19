package hooks

import "encoding/json"

// Action is the PreTool decision. Only Allow / Deny / Modify exist in v1
// (no synthesize).
type Action int

// Action values are the PreTool decisions a hook can return.
const (
	ActionAllow Action = iota
	ActionDeny
	ActionModify // rewrite Input, then continue to Gate / Run
)

func (a Action) String() string {
	switch a {
	case ActionAllow:
		return "allow"
	case ActionDeny:
		return "deny"
	case ActionModify:
		return "modify"
	default:
		return "unknown"
	}
}

// Event is the payload passed to PreTool / PostTool.
// JSON tags match the external command-hook protocol.
type Event struct {
	SessionID string          `json:"session_id"`
	Cwd       string          `json:"cwd"`
	Tool      string          `json:"tool"`
	ToolUseID string          `json:"tool_use_id"`
	Input     json.RawMessage `json:"input"`

	// PostTool fills these; PreTool leaves them empty.
	Output string `json:"output,omitempty"`
	Err    string `json:"error,omitempty"` // tool error text; empty on success
}

// CommandEvent is the payload passed to a KindCommand hook.
type CommandEvent struct {
	SessionID string
	Cwd       string
	Args      []string
}

// CommandResult is returned from a KindCommand hook.
// Empty stdout with exit 0 is a silent success (both fields empty).
type CommandResult struct {
	Submit string // if set, TUI sends this as a user message
	Toast  string // if set, TUI shows this as a success toast
}

// PreResult is returned from PreTool.
type PreResult struct {
	Action  Action          `json:"action"`
	Input   json.RawMessage `json:"input,omitempty"` // required when ActionModify
	Reason  string          `json:"reason,omitempty"`
	Context string          `json:"context,omitempty"` // optional model-facing note
}

// PostResult is returned from PostTool.
type PostResult struct {
	Context string `json:"context,omitempty"`
	Stop    bool   `json:"stop,omitempty"` // stop this agent round (unused until later slices)
	Reason  string `json:"reason,omitempty"`
	Output  string `json:"output,omitempty"`
}
