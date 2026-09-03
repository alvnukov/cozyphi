package controller

import (
	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/questiontool"
	"github.com/alvnukov/cozyphi/internal/voice"
)

// Msg is a UI-thread message. Producers send; Editor.Update applies.
// Share memory by communicating — not the other way around.
type Msg interface {
	isMsg()
}

// SubmitMsg asks the UI to accept a user prompt.
type SubmitMsg struct {
	Text  string
	Media []llm.Media
}

func (SubmitMsg) isMsg() {}

// ModeToggleMsg asks the UI to toggle the build/plan/useplan posture and refresh the
// composer mode label from the controller's new mode.
type ModeToggleMsg struct{}

func (ModeToggleMsg) isMsg() {}

// CancelStreamMsg aborts the in-flight agent stream.
type CancelStreamMsg struct{}

func (CancelStreamMsg) isMsg() {}

// SessionEventMsg carries a session model event from the agent pipeline.
type SessionEventMsg struct{ Event session.Event }

func (SessionEventMsg) isMsg() {}

// PlanUpdatedMsg carries a plan snapshot only after it has been persisted by
// the session manager. The editor applies it on the UI goroutine.
type PlanUpdatedMsg struct{ Plan session.Plan }

func (PlanUpdatedMsg) isMsg() {}

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

// RunEndedMsg reports that the run pipeline went idle: no run and no queued
// prompt is left. Footers drop run-derived activity on it; outcome states a
// user still reads (Stopped) survive.
type RunEndedMsg struct{}

func (RunEndedMsg) isMsg() {}

// MentionResultsMsg delivers async @-file search results to the UI goroutine.
type MentionResultsMsg struct {
	Gen     int
	Query   string
	Paths   []string
	ErrText string
}

func (MentionResultsMsg) isMsg() {}

// VoiceStateMsg reports a voice dialog transition or meter tick. Gen
// identifies the mode, so a tick from a discarded one is dropped. Pending
// counts the segments queued and in flight; Starting says the capture is
// being (re)opened.
type VoiceStateMsg struct {
	Gen      int
	State    voice.State
	Level    float64
	Pending  int
	Starting bool
}

func (VoiceStateMsg) isMsg() {}

// VoiceResultMsg delivers one finished segment transcript to the composer.
// Seq is the segment's position within the mode.
type VoiceResultMsg struct {
	Gen      int
	Seq      int
	Text     string
	Language string
}

func (VoiceResultMsg) isMsg() {}

// VoiceErrorMsg reports a voice failure as one sentence plus the next action.
type VoiceErrorMsg struct {
	Gen  int
	Seq  int
	Text string
	Hint string
}

func (VoiceErrorMsg) isMsg() {}

// VoiceNoticeMsg reports something worth saying that is not a failure, such
// as the max_seconds auto-stop.
type VoiceNoticeMsg struct {
	Gen  int
	Text string
}

func (VoiceNoticeMsg) isMsg() {}

// PermissionAskMsg asks the UI to confirm a gated tool call.
// Reply must be buffered(1); the UI sends AskReply once.
type PermissionAskMsg struct {
	Request permission.Request
	Reason  string
	Reply   chan AskReply

	// PersistPath is the config file an Allow-All-for-Every-Session choice
	// would write, so the overlay can name it instead of promising an
	// unnamed rule in an unnamed place.
	PersistPath string
}

func (PermissionAskMsg) isMsg() {}

// PermissionPersistedMsg reports the outcome of writing the persistent
// allow-all rule, so the user knows where it lives — or that it does not.
type PermissionPersistedMsg struct {
	Path    string
	ErrText string
}

func (PermissionPersistedMsg) isMsg() {}

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

// QuestionAskMsg asks the UI to render an interactive question and wait for
// the user to pick from options. Reply must be buffered(1); the UI sends
// QuestionReply once.
type QuestionAskMsg struct {
	Questions []questiontool.Question
	Reply     chan QuestionReply
}

func (QuestionAskMsg) isMsg() {}

// QuestionDismissMsg clears a pending question overlay (timeout/cancel).
type QuestionDismissMsg struct{}

func (QuestionDismissMsg) isMsg() {}

// UpdateAvailableMsg delivers a startup version-check result to the UI.
type UpdateAvailableMsg struct {
	Latest  string
	Current string
}

func (UpdateAvailableMsg) isMsg() {}

// HookCommandResultMsg delivers the result of a KindCommand hook slash command.
type HookCommandResultMsg struct {
	Gen       uint64
	Submit    string
	Toast     string
	Status    string
	StatusSet bool
	List      *hooks.CommandList
	Err       string
}

func (HookCommandResultMsg) isMsg() {}

// HookSessionEffectsMsg applies toast/status from session lifecycle hooks.
type HookSessionEffectsMsg struct {
	Toast     string
	Status    string
	StatusSet bool
}

func (HookSessionEffectsMsg) isMsg() {}

// NotifierFailedMsg reports that the desktop notification sender failed and
// notifications are off for this session, so the silence has an explanation.
type NotifierFailedMsg struct {
	ErrText string
}

func (NotifierFailedMsg) isMsg() {}

// JobProgressMsg carries a live sub-agent tool update for the nested tree UI.
type JobProgressMsg struct {
	Progress job.Progress
}

func (JobProgressMsg) isMsg() {}

// BranchLabelMsg refreshes the path label's git branch after an external
// checkout (e.g. from another terminal or editor).
type BranchLabelMsg struct {
	Text string
}

func (BranchLabelMsg) isMsg() {}

// ProviderCatalogMsg delivers a background catalog refresh to the UI thread.
type ProviderCatalogMsg struct {
	Providers []provider.Info
	ErrText   string
}

func (ProviderCatalogMsg) isMsg() {}

// ProviderDeviceCodeMsg displays a subscription sign-in code without OAuth tokens.
type ProviderDeviceCodeMsg struct {
	ProviderID      string
	VerificationURL string
	UserCode        string
	ErrText         string
}

func (ProviderDeviceCodeMsg) isMsg() {}

// ProviderAuthorizationMsg displays a browser OAuth URL without credentials.
type ProviderAuthorizationMsg struct {
	ProviderID       string
	AuthorizationURL string
	BrowserErrText   string
	ErrText          string
}

func (ProviderAuthorizationMsg) isMsg() {}

// ProviderConnectResultMsg reports credential persistence without carrying the secret.
type ProviderConnectResultMsg struct {
	ProviderID  string
	ErrText     string
	WarningText string
}

func (ProviderConnectResultMsg) isMsg() {}

// ProviderModelsUpdatedMsg reports completion of a background subscription
// model refresh without exposing credentials or response bodies.
type ProviderModelsUpdatedMsg struct {
	ErrText string
}

func (ProviderModelsUpdatedMsg) isMsg() {}
