package submit

import (
	"strings"
	"time"

	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tui/commands"
	"github.com/pulseaiclub/phi/internal/tui/composer"
	"github.com/pulseaiclub/phi/internal/tui/controller"
	"github.com/pulseaiclub/phi/internal/tui/transcript"
)

// Submitter owns submit / cancel / slash dispatch and coordinates bash runs.
type Submitter struct {
	ctrl       *controller.Controller
	commands   *commands.CommandRegistry
	transcript *transcript.TranscriptPane
	activity   *controller.ActivityHandler
	composer   composer.Input
	bash       *BashRunner

	commandContext func() commands.CommandContext
	publish        func(controller.Msg)

	permissionActive  func() bool
	continueActive    func() bool
	resolvePermission func(controller.AskReply)
	resolveContinue   func(controller.ContinueReply)
}

// NewSubmitter builds a Submitter from explicit collaborators (no *Editor back-pointer).
func NewSubmitter(
	ctrl *controller.Controller,
	commands *commands.CommandRegistry,
	transcript *transcript.TranscriptPane,
	activity *controller.ActivityHandler,
	composer composer.Input,
	bash *BashRunner,
	commandContext func() commands.CommandContext,
	publish func(controller.Msg),
	permissionActive func() bool,
	continueActive func() bool,
	resolvePermission func(controller.AskReply),
	resolveContinue func(controller.ContinueReply),
) *Submitter {
	return &Submitter{
		ctrl:              ctrl,
		commands:          commands,
		transcript:        transcript,
		activity:          activity,
		composer:          composer,
		bash:              bash,
		commandContext:    commandContext,
		publish:           publish,
		permissionActive:  permissionActive,
		continueActive:    continueActive,
		resolvePermission: resolvePermission,
		resolveContinue:   resolveContinue,
	}
}

// Bash returns the local shell runner owned by this submitter.
func (s *Submitter) Bash() *BashRunner {
	if s == nil {
		return nil
	}
	return s.bash
}

// SyncBashBorder updates composer chrome for "!cmd" prefix.
func (s *Submitter) SyncBashBorder(text string) {
	if s == nil || s.bash == nil {
		return
	}
	s.bash.SyncBorder(text)
}

// Submit handles a user prompt from the composer (agent, slash, or bash).
func (s *Submitter) Submit(text string) {
	if s == nil {
		return
	}
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "!") {
		if s.bash != nil && s.bash.HandleSubmit(text) {
			return
		}
	} else if strings.HasPrefix(text, "/") {
		if s.dispatchSlash(text) {
			s.composer.HideCompleters()
			s.composer.ClearInput()
			s.composer.SyncBashBorder("")
			return
		}
	} else {
		s.handleUserInput(text)
	}
}

func (s *Submitter) handleUserInput(text string) {
	pendingSkills := s.composer.PendingSkills()
	if (text == "" && len(pendingSkills) == 0) || s.IsBusy() {
		return
	}

	s.composer.CloseMentionSlash()

	s.activity.Apply(controller.ActivitySubmitting)
	display := text
	if display == "" && len(pendingSkills) > 0 {
		display = "Skills: " + strings.Join(pendingSkills, ", ")
	}
	s.transcript.ApplySession(session.UserAppend{Text: display})
	s.transcript.Sync()
	s.transcript.StickToBottom()

	s.activity.Apply(controller.ActivityWaiting)

	s.composer.ClearInput()
	s.composer.ClearPendingSkills()

	if s.ctrl != nil {
		s.ctrl.StartPrompt(text, pendingSkills)
	}
}

// Cancel aborts overlays, bash, or the in-flight agent stream.
func (s *Submitter) Cancel() {
	if s == nil {
		return
	}
	if s.resolvePermission != nil && s.permissionActive != nil && s.permissionActive() {
		s.resolvePermission(controller.AskReply{})
	}
	if s.resolveContinue != nil && s.continueActive != nil && s.continueActive() {
		s.resolveContinue(controller.ContinueReply{})
	}
	if s.bash != nil && s.bash.Cancel() {
		return
	}
	if s.ctrl != nil {
		s.ctrl.Cancel()
	}
	s.transcript.ApplySession(session.CancelStreaming{})
	s.transcript.Sync()
	s.activity.Apply(controller.ActivityCancelled)
	if s.publish != nil {
		time.AfterFunc(1200*time.Millisecond, func() {
			s.publish(controller.ClearIfActivityMsg{If: controller.ActivityCancelled})
		})
	}
}

// RunningBash reports whether a local "!cmd" is in flight.
func (s *Submitter) RunningBash() bool {
	if s == nil || s.bash == nil {
		return false
	}
	return s.bash.Running()
}

// IsBusy reports agent stream or local bash activity.
func (s *Submitter) IsBusy() bool {
	if s == nil {
		return false
	}
	if s.transcript != nil && s.transcript.IsStreaming() {
		return true
	}
	return s.bash != nil && s.bash.Running()
}

// StreamActive reports whether user input should be blocked for stream/overlays.
func (s *Submitter) StreamActive() bool {
	if s == nil {
		return false
	}
	if s.IsBusy() ||
		(s.permissionActive != nil && s.permissionActive()) ||
		(s.continueActive != nil && s.continueActive()) {
		return true
	}
	if s.activity == nil {
		return false
	}
	switch s.activity.Current {
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
	if s.commands == nil || s.commandContext == nil {
		return false
	}
	return s.commands.DispatchSlash(text, s.commandContext())
}
