package editor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/tui/composer"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/overlays"
	"github.com/alvnukov/cozyphi/internal/tui/submit"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

// newInterruptEditor builds the slice of the editor AcceptInterrupt walks: the
// overlays, the submitter and the composer draft, with nothing in flight.
func newInterruptEditor(t *testing.T) *Editor {
	t.Helper()
	th := components.DefaultTheme()
	return &Editor{
		overlays: overlays.NewOverlays(th, nil, nil, nil, nil),
		composer: composer.NewComposerPane(th, "m", t.TempDir(), nil),
	}
}

// newCancelSpy builds a submitter that reports work in flight — a pending ask
// blocks submission — and records the cancellation it receives.
func newCancelSpy(t *testing.T, cancelled *bool) *submit.Submitter {
	t.Helper()
	th := components.DefaultTheme()
	tp := transcript.NewTranscriptPane(th, status.NewSpinner(th.ToolName), "CozyPhi test")
	return submit.NewSubmitter(nil, nil, tp, nil, nil, nil, nil, nil,
		func() bool { return true }, nil,
		func(controller.AskReply) { *cancelled = true }, nil)
}

// The innermost thing in flight goes first: a pending ask is declined, and the
// run behind it survives for the next press.
func TestAcceptInterruptDeclinesAskBeforeCancellingRun(t *testing.T) {
	e := newInterruptEditor(t)
	reply := make(chan controller.AskReply, 1)
	e.overlays.Apply(controller.PermissionAskMsg{Reply: reply})
	cancelled := false
	e.submitter = newCancelSpy(t, &cancelled)

	require.True(t, e.AcceptInterrupt(), "an interrupt keeps the app up")
	assert.False(t, e.overlays.Active(), "the ask is gone")
	select {
	case r := <-reply:
		assert.Equal(t, controller.AskReply{}, r, "dismissing an ask denies it")
	default:
		t.Fatal("the ask must be answered, not orphaned")
	}
	assert.False(t, cancelled, "the run behind the ask is left for the next press")
}

// With no overlay left, the press cancels the work behind it.
func TestAcceptInterruptCancelsRunBeforeClearingDraft(t *testing.T) {
	e := newInterruptEditor(t)
	cancelled := false
	e.submitter = newCancelSpy(t, &cancelled)
	e.composer.Chat.Value = "half a prompt"

	require.True(t, e.AcceptInterrupt())
	assert.True(t, cancelled, "the run is cancelled")
	assert.Equal(t, "half a prompt", e.composer.Chat.Value, "the draft outlives the run")
}

// Once the session is idle the draft is the last thing left to lose, and
// losing it is not the exit hint: the app is still not armed to quit.
func TestAcceptInterruptClearsDraftWhenIdle(t *testing.T) {
	e := newInterruptEditor(t)
	e.composer.Chat.Value = "half a prompt"

	require.True(t, e.AcceptInterrupt())
	assert.Empty(t, e.composer.Chat.Value)
	assert.False(t, e.toast.Visible(), "clearing a draft does not arm the exit")
	assert.True(t, e.lastCtrlC.IsZero())
}

// With nothing to interrupt the first press arms the exit and says so; the
// second one inside the window is what quits.
func TestAcceptInterruptArmsExitThenQuits(t *testing.T) {
	e := newInterruptEditor(t)

	require.True(t, e.AcceptInterrupt(), "the first press must not quit")
	assert.True(t, e.toast.Visible())
	assert.Contains(t, e.toast.Message, "Ctrl+C again")
	assert.Equal(t, toast.ToastWarning, e.toast.Kind)
	assert.False(t, e.AcceptInterrupt(), "the second press quits")
}

// The armed exit expires with its hint, so a press long after it arms again
// instead of quitting on a Ctrl+C the user has forgotten pressing.
func TestAcceptInterruptRearmsAfterWindow(t *testing.T) {
	e := newInterruptEditor(t)
	require.True(t, e.AcceptInterrupt())
	e.lastCtrlC = time.Now().Add(-2 * ctrlCExitWindow)

	assert.True(t, e.AcceptInterrupt(), "a stale arming must not quit")
}

// Interrupting something disarms the exit: the count starts over, so stopping
// a run never leaves the app one keypress from dying.
func TestAcceptInterruptDisarmsAfterInterruptingWork(t *testing.T) {
	e := newInterruptEditor(t)
	require.True(t, e.AcceptInterrupt(), "arms the exit")
	e.composer.Chat.Value = "typed while armed"

	require.True(t, e.AcceptInterrupt(), "clears the draft")
	assert.True(t, e.lastCtrlC.IsZero(), "an interrupt disarms the exit")
	assert.True(t, e.AcceptInterrupt(), "the next press arms again instead of quitting")
}
