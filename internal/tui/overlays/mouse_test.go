package overlays

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tools/questiontool"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

// beginTestPermissionAsk opens a bash permission ask and records the panel
// rectangle the way a frame would.
func beginTestPermissionAsk(t *testing.T, o *Overlays) chan controller.AskReply {
	t.Helper()
	reply := make(chan controller.AskReply, 1)
	o.beginPermissionAsk(controller.PermissionAskMsg{
		Request: permission.Request{Tool: "bash", Action: permission.ActionBash, Command: "curl https://x"},
		Reply:   reply,
	})
	drawTestPanel(t, o)
	return reply
}

// drawTestPanel renders the bottom panel at 80 columns and pins its origin
// at (0, 10), so tests can aim clicks in screen coordinates.
func drawTestPanel(t *testing.T, o *Overlays) {
	t.Helper()
	h, overlay := o.PreferredBottomHeight(80, 0)
	if !overlay {
		t.Fatal("expected an overlay to measure")
	}
	if _, ok := o.DrawBottom(components.DrawContext{}, 80, h); !ok {
		t.Fatal("expected the overlay to draw")
	}
	o.SetBottomOrigin(0, 10)
}

// rowContaining is the screen Y of the first panel row whose text carries
// the needle, resolved from the same rows the panel painted.
func rowContaining(t *testing.T, body []components.RichLine, needle string) int {
	t.Helper()
	for i, line := range body {
		if strings.Contains(lineText(line), needle) {
			return 10 + 1 + i
		}
	}
	t.Fatalf("no row contains %q", needle)
	return 0
}

func click(x, y int) xui.MouseEvent {
	return xui.MouseEvent{X: x, Y: y, Button: xui.MouseLeft, Action: xui.MousePress}
}

func TestAskMouseClickOnTheSelectedOptionActivates(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := beginTestPermissionAsk(t, o)
	body, _ := o.perm.askRows(o.theme, askInnerWidth(80), 0)

	ctx := &components.EventContext{}
	if !o.HandleAskMouse(ctx, click(4, rowContaining(t, body, "Approve"))) {
		t.Fatal("expected the ask to take the click")
	}
	if o.perm != nil {
		t.Fatal("expected the ask resolved")
	}
	select {
	case r := <-reply:
		if !r.Approved || r.AllowSession || r.AllowPersistent {
			t.Fatalf("got %+v", r)
		}
	default:
		t.Fatal("expected a reply")
	}
}

func TestAskMouseClickSelectsBeforeItActivates(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := beginTestPermissionAsk(t, o)
	body, _ := o.perm.askRows(o.theme, askInnerWidth(80), 0)
	y := rowContaining(t, body, "Allow All for This Session")

	o.HandleAskMouse(&components.EventContext{}, click(4, y))
	if o.perm == nil || o.perm.ring.Selected() != 1 {
		t.Fatal("the first click must select the option, not activate it")
	}
	select {
	case <-reply:
		t.Fatal("a first click on an unselected option must not resolve")
	default:
	}

	o.HandleAskMouse(&components.EventContext{}, click(4, y))
	select {
	case r := <-reply:
		if !r.Approved || !r.AllowSession {
			t.Fatalf("got %+v", r)
		}
	default:
		t.Fatal("expected the second click to activate")
	}
}

func TestAskMouseOutsideThePanelIsConsumedAndChangesNothing(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	beginTestPermissionAsk(t, o)

	ctx := &components.EventContext{}
	if !o.HandleAskMouse(ctx, click(85, 12)) {
		t.Fatal("a modal must consume every mouse event")
	}
	if !ctx.Consume {
		t.Fatal("expected the event consumed")
	}
	if o.perm == nil || o.perm.ring.Selected() != 0 {
		t.Fatal("a click outside the panel must change nothing")
	}
}

func TestAskMouseWheelStepsTheOptions(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	beginTestPermissionAsk(t, o)

	// The wheel works wherever the pointer is: the modal owns the mouse, so
	// hovering the transcript must not scroll it.
	wheel := xui.MouseEvent{X: 40, Y: 2, Button: xui.MouseWheelDown, Action: xui.MousePress}
	o.HandleAskMouse(&components.EventContext{}, wheel)
	if o.perm.ring.Selected() != 1 {
		t.Fatalf("selected=%d after one wheel notch", o.perm.ring.Selected())
	}
	wheel.Button = xui.MouseWheelUp
	o.HandleAskMouse(&components.EventContext{}, wheel)
	if o.perm.ring.Selected() != 0 {
		t.Fatalf("selected=%d after wheeling back", o.perm.ring.Selected())
	}
}

func TestAskMouseIsInertWhileTypingFeedback(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	beginTestPermissionAsk(t, o)
	o.acceptPermissionOption(askOptDenyFeedback)
	drawTestPanel(t, o)

	ctx := &components.EventContext{}
	if !o.HandleAskMouse(ctx, click(4, 12)) || !ctx.Consume {
		t.Fatal("the modal still owns the mouse in feedback mode")
	}
	if o.perm == nil || !o.perm.feedbackMode {
		t.Fatal("a click must not leave feedback mode")
	}
}

func TestContinueAskMouseClickResolves(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.ContinueReply, 1)
	o.beginContinueAsk(controller.ContinueAskMsg{MaxRounds: 30, Reply: reply})
	drawTestPanel(t, o)
	body, _ := o.cont.askRows(o.theme, askInnerWidth(80), 0)

	// "[1]" pins the Continue option row; the intro row also says Continue.
	o.HandleAskMouse(&components.EventContext{}, click(4, rowContaining(t, body, "[1]")))
	select {
	case r := <-reply:
		if !r.Continue {
			t.Fatalf("got %+v", r)
		}
	default:
		t.Fatal("expected the click on the selected option to resolve")
	}
}

func TestQuestionAskMouseClickPicksTheOption(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.QuestionReply, 1)
	o.beginQuestionAsk(controller.QuestionAskMsg{Questions: []questiontool.Question{{
		Question: "ship it?",
		Header:   "h",
		Options: []questiontool.Option{
			{Label: "yes"},
			{Label: "no", Description: "keep iterating"},
		},
	}}, Reply: reply})
	drawTestPanel(t, o)
	body, _ := o.question.askRows(o.theme, askInnerWidth(80), 0)

	// A description row belongs to its option: clicking it selects option 2.
	y := rowContaining(t, body, "keep iterating")
	o.HandleAskMouse(&components.EventContext{}, click(4, y))
	if o.question == nil || o.question.ring.Selected() != 1 {
		t.Fatal("the description row must select the option above it")
	}

	o.HandleAskMouse(&components.EventContext{}, click(4, y))
	select {
	case r := <-reply:
		if len(r.Answers) != 1 || strings.Join(r.Answers[0], ",") != "no" {
			t.Fatalf("got %+v", r.Answers)
		}
	default:
		t.Fatal("expected the second click to answer the single question")
	}
}
