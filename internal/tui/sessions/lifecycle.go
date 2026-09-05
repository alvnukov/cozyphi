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
	"github.com/alvnukov/cozyphi/internal/tui/submit"
)

// Status is a detached, session-local summary for the shell's session selector.
type Status struct {
	Running     bool
	Waiting     string
	Unread      int
	Error       string
	Interrupted bool
	Stopped     bool
	LiveJobs    int
	Attention   string
}

// UI state, including activation and Close, is owned by the UI goroutine.
type viewLifetime struct {
	active, closed         bool
	focus                  components.Widget
	status                 Status
	identity               string
	ctx                    context.Context
	cancel                 context.CancelFunc
	branchOnce             sync.Once
	branchStop, branchDone chan struct{}
	closeDone              chan struct{}
	closeErr               error
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
		if e.ctrl != nil {
			e.ctrl.LeaveAssignment()
		}
		if e.App != nil && e.App.Focused() != nil {
			e.lifetime.focus = e.App.Focused()
		}
		e.lifetime.active = false
		return
	}
	e.lifetime.active = true
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
	if e.ctrl != nil {
		s.LiveJobs = e.ctrl.LiveJobCount()
		s.Running = s.Running || e.ctrl.RunActive() || s.LiveJobs > 0
		a := e.ctrl.Assignment()
		if a.JobID != "" {
			s.Stopped = a.Terminal && a.StopRequested
			s.Interrupted = !a.Terminal &&
				(a.Turn == controller.TurnInterrupting || a.Turn == controller.TurnInterrupted)
		}
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
		if update, ok := msg.Event.(session.AssistantMessageUpdate); ok {
			if update.Message.State == session.StateError && s.Error == "" {
				s.Error = "Run failed"
				e.recordAttention("error: run failed")
				if e.notifier != nil {
					e.notifier.NeedsAttention("Run failed")
				}
			}
			s.Interrupted = update.Message.State == session.StateCancelled
		}
	case controller.SetActivityMsg:
		switch msg.Activity {
		case controller.ActivitySubmitting, controller.ActivityWaiting, controller.ActivityStreaming,
			controller.ActivityTools, controller.ActivityCompacting, controller.ActivityAwaitingApproval:
			s.Running = true
		}
		if msg.Activity == controller.ActivitySubmitting {
			s.Error = ""
			s.Interrupted, s.Stopped = false, false
		}
	case controller.RunEndedMsg:
		s.Running, s.Waiting = false, ""
		s.Unread++
		if s.Error != "" {
			e.recordAttention("error: run failed")
		} else {
			e.recordAttention("turn ended")
		}
	case controller.PermissionAskMsg:
		s.Waiting = "permission"
		e.recordAttention(msg.Request.Tool + " waiting for permission")
	case controller.ContinueAskMsg:
		s.Waiting = "continue"
		e.recordAttention("waiting to continue")
	case controller.QuestionAskMsg:
		s.Waiting = "question"
		e.recordAttention("question: " + questionDetail(msg.Questions))
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

// bindBashLifetime runs before submission is exposed. The captured runner, not
// mutable UI state, is joined by every controller/runtime disposal route.
func (e *View) bindBashLifetime(runner *submit.BashRunner) {
	e.bashRunner = runner
	if e.ctrl == nil {
		return // UI-only assemblies are disposed by View.Close.
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	if !e.ctrl.TrackLifetime(cancel, done) {
		cancel()
		// No submissions have been admitted yet; reject them synchronously.
		_ = runner.Close(context.Background())
		close(done)
		return
	}
	go func() {
		defer close(done)
		<-ctx.Done()
		// An unbounded Close can only succeed; its completion includes publication.
		_ = runner.Close(context.Background())
	}()
}

// Close denies new UI work and cancels owned work. Cleanup outlives a timed-out
// caller and retains history until local shell exit and final publication.
// The controller owns its session, not the Runtime borrowed from the parent.
func (e *View) Close(ctx context.Context) error {
	if e == nil {
		return nil
	}
	e.BeginClose()
	return e.awaitClose(ctx)
}

// BeginClose runs the UI-goroutine half of Close: it retires the view and
// starts cleanup without waiting. The shell calls it for every view before
// waiting on all of them at once, so the wait never serializes per view.
func (e *View) BeginClose() {
	if e == nil {
		return
	}
	if !e.lifetime.closed {
		e.SetActive(false)
		e.lifetime.closed = true
		if e.lifetime.cancel != nil {
			e.lifetime.cancel()
		}
		e.CloseVoice()
		if e.settingsDetach != nil {
			e.settingsDetach()
		}
		if e.overlays != nil {
			e.overlays.CancelActive()
		}
		if e.lifetime.branchStop != nil {
			close(e.lifetime.branchStop)
		}
		// Cancel the stream before joining shell publication: a publisher may
		// need it to stop. The registered barrier also covers independent closes.
		if e.ctrl != nil {
			e.ctrl.Cancel()
		}
		e.lifetime.closeDone = make(chan struct{})
		go func() {
			defer close(e.lifetime.closeDone)
			e.lifetime.closeErr = e.bashRunner.Close(context.Background())
			if e.statusHistory != nil {
				e.statusHistory.Close()
			}
			if e.ctrl != nil {
				e.ctrl.Close()
			}
		}()
	}
}

// awaitClose joins cleanup started by BeginClose. Work that already finished
// is reported as such even when ctx has expired: a select with both cases
// ready would otherwise pick the deadline at random.
func (e *View) awaitClose(ctx context.Context) error {
	if err := awaitDone(ctx, e.lifetime.closeDone); err != nil {
		return fmt.Errorf("close view: %w", err)
	}
	if e.lifetime.branchDone != nil {
		if err := awaitDone(ctx, e.lifetime.branchDone); err != nil {
			return fmt.Errorf("close branch watch: %w", err)
		}
	}
	if e.lifetime.closeErr != nil {
		return fmt.Errorf("close local shell: %w", e.lifetime.closeErr)
	}
	if e.ctrl != nil && (e.ctrl.RunActive() || e.ctrl.LiveJobCount() > 0) {
		return errors.New("close view: session tools are still stopping; retry Close")
	}
	return nil
}

func awaitDone(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
