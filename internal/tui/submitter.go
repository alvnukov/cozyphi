package tui

import (
	"strings"
	"time"

	"github.com/pulseaiclub/phi/internal/components/toast"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tui/controller"
)

// SubmitterDeps wires explicit collaborators for Submitter (no *Editor back-pointer).
type SubmitterDeps struct {
	Ctrl       *controller.Controller
	Bus        *controller.Bus
	Commands   *CommandRegistry
	Transcript *TranscriptPane
	Activity   *controller.ActivityHandler
	Composer   submitComposer
	Bash       *BashRunner

	CommandContext func() CommandContext
	Toast          func(msg string, kind toast.ToastKind, d time.Duration)
	Publish        func(controller.Msg)

	PermissionActive  func() bool
	ContinueActive    func() bool
	ResolvePermission func(controller.AskReply)
	ResolveContinue   func(controller.ContinueReply)
}

// Submitter owns submit / cancel / slash dispatch and coordinates bash runs.
type Submitter struct {
	deps SubmitterDeps
}

// NewSubmitter builds a Submitter from explicit dependencies.
func NewSubmitter(deps SubmitterDeps) *Submitter {
	return &Submitter{deps: deps}
}

// Bash returns the local shell runner owned by this submitter.
func (s *Submitter) Bash() *BashRunner {
	if s == nil {
		return nil
	}
	return s.deps.Bash
}

// SyncBashBorder updates composer chrome for "!cmd" prefix.
func (s *Submitter) SyncBashBorder(text string) {
	if s == nil || s.deps.Bash == nil {
		return
	}
	s.deps.Bash.SyncBorder(text)
}

// Submit handles a user prompt from the composer (agent, slash, or bash).
func (s *Submitter) Submit(text string) {
	if s == nil {
		return
	}
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "!") {
		if s.deps.Bash != nil && s.deps.Bash.HandleSubmit(text) {
			return
		}
	}
	if strings.HasPrefix(text, "/") {
		if s.dispatchSlash(text) {
			s.deps.Composer.HideCompleters()
			s.deps.Composer.ClearInput()
			s.deps.Composer.SyncBashBorder("")
			return
		}
	}
	pendingSkills := s.deps.Composer.PendingSkills()
	if (text == "" && len(pendingSkills) == 0) || s.IsBusy() {
		return
	}

	s.deps.Composer.CloseMentionSlash()

	if s.deps.Activity != nil {
		s.deps.Activity.Apply(controller.ActivitySubmitting)
	}
	display := text
	if display == "" && len(pendingSkills) > 0 {
		display = "Skills: " + strings.Join(pendingSkills, ", ")
	}
	s.deps.Transcript.ApplySession(session.UserAppend{Text: display})
	s.deps.Transcript.Sync()
	s.deps.Transcript.StickToBottom()
	if s.deps.Activity != nil {
		s.deps.Activity.Apply(controller.ActivityWaiting)
	}

	s.deps.Composer.ClearInput()
	s.deps.Composer.ClearPendingSkills()

	if s.deps.Ctrl != nil {
		s.deps.Ctrl.StartPrompt(text, pendingSkills)
	}
}

// Cancel aborts overlays, bash, or the in-flight agent stream.
func (s *Submitter) Cancel() {
	if s == nil {
		return
	}
	if s.deps.ResolvePermission != nil && s.permissionActive() {
		s.deps.ResolvePermission(controller.AskReply{})
	}
	if s.deps.ResolveContinue != nil && s.continueActive() {
		s.deps.ResolveContinue(controller.ContinueReply{})
	}
	if s.deps.Bash != nil && s.deps.Bash.Cancel() {
		return
	}
	if s.deps.Ctrl != nil {
		s.deps.Ctrl.Cancel()
	}
	s.deps.Transcript.ApplySession(session.CancelStreaming{})
	s.deps.Transcript.Sync()
	if s.deps.Activity != nil {
		s.deps.Activity.Apply(controller.ActivityCancelled)
	}
	if s.deps.Publish != nil {
		time.AfterFunc(1200*time.Millisecond, func() {
			s.deps.Publish(controller.ClearIfActivityMsg{If: controller.ActivityCancelled})
		})
	}
}

// RunningBash reports whether a local "!cmd" is in flight.
func (s *Submitter) RunningBash() bool {
	if s == nil || s.deps.Bash == nil {
		return false
	}
	return s.deps.Bash.Running()
}

// IsBusy reports agent stream or local bash activity.
func (s *Submitter) IsBusy() bool {
	if s == nil {
		return false
	}
	if s.deps.Transcript != nil && s.deps.Transcript.IsStreaming() {
		return true
	}
	return s.deps.Bash != nil && s.deps.Bash.Running()
}

// StreamActive reports whether user input should be blocked for stream/overlays.
func (s *Submitter) StreamActive() bool {
	if s == nil {
		return false
	}
	if s.IsBusy() || s.permissionActive() || s.continueActive() {
		return true
	}
	if s.deps.Activity == nil {
		return false
	}
	switch s.deps.Activity.Current {
	case controller.ActivitySubmitting,
		controller.ActivityWaiting,
		controller.ActivityStreaming,
		controller.ActivityTools,
		controller.ActivityCompacting,
		controller.ActivityAwaitingApproval,
		controller.ActivityRetrying:
		return true
	default:
		return false
	}
}

func (s *Submitter) dispatchSlash(text string) bool {
	if s.deps.Commands == nil || s.deps.CommandContext == nil {
		return false
	}
	return s.deps.Commands.DispatchSlash(text, s.deps.CommandContext())
}

func (s *Submitter) permissionActive() bool {
	return s.deps.PermissionActive != nil && s.deps.PermissionActive()
}

func (s *Submitter) continueActive() bool {
	return s.deps.ContinueActive != nil && s.deps.ContinueActive()
}
