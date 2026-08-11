package tui

import (
	"github.com/pulseaiclub/phi/internal/job"
	"github.com/pulseaiclub/phi/internal/permission"
	"github.com/pulseaiclub/phi/internal/session"
)

// Msg is a UI-thread message. Producers send; Editor.Update applies.
// Share memory by communicating — not the other way around.
type Msg interface {
	isMsg()
}

// SubmitMsg asks the UI to accept a user prompt.
type SubmitMsg struct{ Text string }

func (SubmitMsg) isMsg() {}

// CancelStreamMsg aborts the in-flight agent stream.
type CancelStreamMsg struct{}

func (CancelStreamMsg) isMsg() {}

// SessionEventMsg carries a session model event from the agent pipeline.
type SessionEventMsg struct{ Event session.Event }

func (SessionEventMsg) isMsg() {}

// SetActivityMsg sets footer/stream activity status.
type SetActivityMsg struct{ Activity Activity }

func (SetActivityMsg) isMsg() {}

// RedrawMsg only asks for a frame (e.g. delayed activity clear).
type RedrawMsg struct{}

func (RedrawMsg) isMsg() {}

// ClearIfActivityMsg sets Idle only when current activity still matches If.
// Used for delayed "Stopped" → Idle without clobbering a newer state.
type ClearIfActivityMsg struct{ If Activity }

func (ClearIfActivityMsg) isMsg() {}

// MentionResultsMsg delivers async @-file search results to the UI goroutine.
type MentionResultsMsg struct {
	Gen     int
	Query   string
	Paths   []string
	ErrText string
}

func (MentionResultsMsg) isMsg() {}

// PermissionAskMsg asks the UI to confirm a gated tool call.
// Reply must be buffered(1); the UI sends AskReply once.
type PermissionAskMsg struct {
	Request permission.Request
	Reason  string
	Reply   chan AskReply
}

func (PermissionAskMsg) isMsg() {}

// PermissionDismissMsg clears a pending permission overlay (timeout/cancel).
type PermissionDismissMsg struct{}

func (PermissionDismissMsg) isMsg() {}

// ContinueAskMsg asks the UI whether to grant another max-rounds budget.
// Reply must be buffered(1); the UI sends ContinueReply once.
type ContinueAskMsg struct {
	MaxRounds int
	Reply     chan ContinueReply
}

func (ContinueAskMsg) isMsg() {}

// ContinueDismissMsg clears a pending continue overlay (timeout/cancel).
type ContinueDismissMsg struct{}

func (ContinueDismissMsg) isMsg() {}

// UpdateAvailableMsg delivers a startup version-check result to the UI.
type UpdateAvailableMsg struct {
	Latest  string
	Current string
}

func (UpdateAvailableMsg) isMsg() {}

// JobProgressMsg carries a live sub-agent tool update for the nested tree UI.
type JobProgressMsg struct {
	Progress job.Progress
}

func (JobProgressMsg) isMsg() {}
