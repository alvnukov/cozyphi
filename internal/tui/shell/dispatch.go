package shell

import (
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/toast"
	"github.com/pulseaiclub/phi/internal/tui/controller"
)

// Publish sends a message onto the bus from any goroutine / widget callback.
func (sh *Shell) Publish(m controller.Msg) {
	if sh.bus == nil {
		return
	}
	sh.bus.Publish(m)
}

// Update applies one message on the UI goroutine.
func (sh *Shell) Update(m controller.Msg) {
	switch msg := m.(type) {
	case controller.SubmitMsg:
		sh.submitter.Submit(msg.Text)
	case controller.CancelStreamMsg:
		sh.submitter.Cancel()
	case controller.MentionResultsMsg:
		sh.composer.ApplyMentionResults(msg)
	case controller.PermissionAskMsg, controller.PermissionDismissMsg,
		controller.ContinueAskMsg, controller.ContinueDismissMsg:
		sh.overlays.Apply(m)
	case controller.SetActivityMsg, controller.ClearIfActivityMsg, controller.UpdateAvailableMsg:
		sh.footer.Apply(m)
	case controller.HookSessionEffectsMsg:
		sh.footer.Apply(m)
		if msg.Toast != "" {
			sh.toast.Show(msg.Toast, toast.ToastSuccess, 3*time.Second)
		}
	case controller.BranchLabelMsg:
		sh.composer.SetBranchLabel(msg.Text)
		if sh.vx != nil {
			sh.vx.QueueRefresh()
		}
	case controller.HookCommandResultMsg:
		if sh.hookCmds != nil {
			sh.hookCmds.Apply(msg)
		}
	case controller.JobProgressMsg:
		// Applied in drainBus so we can skip Sync when the tree is unchanged.
	case controller.RedrawMsg:
		// no state change; drain already requested redraw
	}
}

func (sh *Shell) drainBus() {
	batch := sh.bus.Drain()
	if len(batch) == 0 {
		return
	}
	atBottom := sh.transcript.AtBottom()
	threadDirty := false
	for _, m := range batch {
		switch msg := m.(type) {
		case controller.SessionEventMsg:
			threadDirty = true
			sh.transcript.ApplySession(msg.Event)
		case controller.JobProgressMsg:
			if sh.transcript.ApplyJobProgress(msg.Progress) {
				threadDirty = true
			}
		default:
			sh.Update(m)
		}
	}
	if threadDirty {
		sh.transcript.Sync()
		sh.footer.SyncFromSnap(sh.transcript.Snapshot())
		if atBottom {
			sh.transcript.StickToBottom()
		}
	}
}

func (sh *Shell) Handle(ctx *components.EventContext, ev xui.Event) {
	sh.composer.Handle(ctx, ev)
}

func (sh *Shell) handleCopyKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	return sh.transcript.HandleCopyKey(ctx, e)
}

// Draw renders via ShellLayout.
func (sh *Shell) Draw(ctx components.DrawContext) components.Surface {
	return sh.layout.Draw(ctx)
}

func (sh *Shell) requestRedraw() {
	if sh.App != nil {
		sh.App.RequestRedraw()
	}
}

// RequestRedraw asks the app to repaint (safe to bind onto controller.RedrawRelay / controller.Bus).
func (sh *Shell) RequestRedraw() {
	sh.requestRedraw()
}
