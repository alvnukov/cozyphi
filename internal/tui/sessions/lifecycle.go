package sessions

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

// Status is a detached, session-local summary for the shell's session selector.
type Status struct {
	Running bool
	Waiting string
	Unread  int
	Error   string
}

// UI state, including activation and Close, is owned by the UI goroutine.
type viewLifetime struct {
	active, closed         bool
	focus                  components.Widget
	status                 Status
	ctx                    context.Context
	cancel                 context.CancelFunc
	branchOnce             sync.Once
	branchStop, branchDone chan struct{}
	closeDone              chan struct{}
}

// Active reports whether this view is selected and eligible for application focus.
func (e *View) Active() bool { return e != nil && e.lifetime.active && !e.lifetime.closed }

// SetActive retains the widget graph and logical focus. Selection alone never
// persists preferences; only the selected view installs the global key profile.
func (e *View) SetActive(active bool) {
	if e == nil || e.lifetime.closed || active == e.lifetime.active {
		return
	}
	if !active {
		if e.App != nil && e.App.Focused() != nil {
			e.lifetime.focus = e.App.Focused()
		}
		e.lifetime.active = false
		return
	}
	e.lifetime.active = true
	e.lifetime.status.Unread = 0
	if e.composer != nil {
		if err := keys.SetProfile(e.composer.Chat.EditingMode()); err != nil {
			e.Toast("Cannot activate keymap: "+err.Error(), toast.ToastWarning, 6*time.Second)
		}
	}
	if e.overlays.Active() {
		overlayComposer{e}.HideCompleters()
		overlayComposer{e}.HidePalette()
		e.focusOverlay()
	} else {
		w := e.lifetime.focus
		if w == nil && e.composer != nil {
			w = &e.composer.Chat
		}
		e.requestFocus(w)
	}
	e.RequestRedraw()
}

func (e *View) requestFocus(w components.Widget) {
	e.lifetime.focus = w
	if e.Active() && e.App != nil {
		e.App.RequestFocus(w)
	}
}

// Background asks must not replace the saved palette's focus or hide it.
func (e *View) focusOverlay() {
	if e.Active() {
		e.FocusEditor()
	}
}

func (e *View) restoreOverlayFocus() {
	if !e.overlays.Active() {
		e.lifetime.status.Waiting = ""
	}
	if e.Active() && e.composer != nil {
		e.composer.FocusChat()
	}
}

type overlayComposer struct{ view *View }

func (c overlayComposer) HideCompleters() {
	if c.view.Active() && c.view.composer != nil {
		c.view.composer.HideCompleters()
	}
}

func (c overlayComposer) HidePalette() {
	if c.view.Active() && c.view.composer != nil {
		c.view.composer.HidePalette()
	}
}

// Status never infers that tools stopped just because Close's wait expired.
func (e *View) Status() Status {
	if e == nil {
		return Status{}
	}
	s := e.lifetime.status
	if e.ctrl != nil && (e.ctrl.RunActive() || e.ctrl.LiveJobCount() > 0) {
		s.Running = true
	}
	if e.bashRunner != nil && e.bashRunner.Running() {
		s.Running = true
	}
	return s
}

// The overlay package still has one slot per ask kind, without request-generation
// matching. Multiple/replaced asks and stale dismissals are the next lifecycle step.
func (e *View) recordStatus(m controller.Msg) {
	s := &e.lifetime.status
	switch msg := m.(type) {
	case controller.SessionEventMsg:
		if update, ok := msg.Event.(session.AssistantMessageUpdate); ok && update.Message.State == session.StateError {
			s.Error = "Run failed"
		}
	case controller.SetActivityMsg:
		switch msg.Activity {
		case controller.ActivitySubmitting, controller.ActivityWaiting, controller.ActivityStreaming,
			controller.ActivityTools, controller.ActivityCompacting, controller.ActivityAwaitingApproval:
			s.Running = true
		}
		if msg.Activity == controller.ActivitySubmitting {
			s.Error = ""
		}
	case controller.RunEndedMsg:
		s.Running, s.Waiting = false, ""
		if !e.Active() {
			s.Unread++
		}
	case controller.PermissionAskMsg:
		s.Waiting = "permission"
	case controller.ContinueAskMsg:
		s.Waiting = "continue"
	case controller.QuestionAskMsg:
		s.Waiting = "question"
	case controller.PermissionDismissMsg, controller.ContinueDismissMsg, controller.QuestionDismissMsg:
		s.Waiting = ""
	case controller.ProviderCatalogMsg:
		if msg.ErrText != "" {
			s.Error = msg.ErrText
		}
	case controller.ProviderConnectResultMsg:
		if msg.ErrText != "" {
			s.Error = msg.ErrText
		}
	case controller.ProviderDeviceCodeMsg:
		if msg.ErrText != "" {
			s.Error = msg.ErrText
		}
	case controller.ProviderAuthorizationMsg:
		if msg.ErrText != "" {
			s.Error = msg.ErrText
		}
	case controller.ProviderModelsUpdatedMsg:
		if msg.ErrText != "" {
			s.Error = msg.ErrText
		}
	case controller.VoiceErrorMsg:
		s.Error = msg.Text
	case controller.PermissionPersistedMsg:
		if msg.ErrText != "" {
			s.Error = msg.ErrText
		}
	}
}

// Close denies new UI work and cancels owned work. A timed-out caller can retry;
// publications stay connected so accepted local shell output is not discarded.
// The controller owns its session, not the Runtime borrowed from the parent.
func (e *View) Close(ctx context.Context) error {
	if e == nil {
		return nil
	}
	if !e.lifetime.closed {
		e.SetActive(false)
		e.lifetime.closed = true
		if e.lifetime.cancel != nil {
			e.lifetime.cancel()
		}
		e.CloseVoice()
		if e.overlays != nil {
			e.overlays.CancelActive()
		}
		if e.lifetime.branchStop != nil {
			close(e.lifetime.branchStop)
		}
		e.lifetime.closeDone = make(chan struct{})
		go func() {
			defer close(e.lifetime.closeDone)
			if e.statusHistory != nil {
				e.statusHistory.Close()
			}
			if e.ctrl != nil {
				e.ctrl.Close()
			}
		}()
	}
	shellErr := e.bashRunner.Close(ctx)
	select {
	case <-e.lifetime.closeDone:
	case <-ctx.Done():
		return fmt.Errorf("close view: %w", ctx.Err())
	}
	if e.lifetime.branchDone != nil {
		select {
		case <-e.lifetime.branchDone:
		case <-ctx.Done():
			return fmt.Errorf("close branch watch: %w", ctx.Err())
		}
	}
	if shellErr != nil {
		return fmt.Errorf("close local shell: %w", shellErr)
	}
	if e.ctrl != nil && (e.ctrl.RunActive() || e.ctrl.LiveJobCount() > 0) {
		return errors.New("close view: session tools are still stopping; retry Close")
	}
	return nil
}
