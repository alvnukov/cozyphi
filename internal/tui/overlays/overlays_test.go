package overlays

import (
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tools/questiontool"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

func testOverlays(activity *controller.ActivityHandler) *Overlays {
	return NewOverlays(components.DefaultTheme(), activity, nil, nil, nil)
}

func TestResolvePermissionSendsReply(t *testing.T) {
	activity := controller.NewActivityHandler(nil)
	o := testOverlays(activity)
	reply := make(chan controller.AskReply, 1)
	o.beginPermissionAsk(controller.PermissionAskMsg{
		Request: permission.Request{Action: permission.ActionBash, Tool: "bash", Command: "curl x"},
		Reason:  "needs approval",
		Reply:   reply,
	})
	if o.perm == nil {
		t.Fatal("expected permAsk")
	}
	if o.perm.header != "Run this command?" {
		t.Fatalf("header=%q", o.perm.header)
	}
	if activity.Current != controller.ActivityAwaitingApproval {
		t.Fatalf("activity=%v", activity.Current)
	}
	o.resolvePermission(controller.AskReply{Approved: true})
	if o.perm != nil {
		t.Fatal("expected cleared")
	}
	select {
	case r := <-reply:
		if !r.Approved {
			t.Fatal("want approved")
		}
	default:
		t.Fatal("expected reply")
	}
}

func TestPermissionAskEscapeCancels(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.AskReply, 1)
	o.beginPermissionAsk(controller.PermissionAskMsg{
		Request: permission.Request{Tool: "bash", Action: permission.ActionBash, Command: "curl https://x"},
		Reply:   reply,
	})
	ctx := &components.EventContext{}
	if !o.handlePermissionKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEscape}) {
		t.Fatal("expected consume")
	}
	if o.perm != nil {
		t.Fatal("expected overlay closed")
	}
	select {
	case r := <-reply:
		if r.Approved || r.Feedback != "" {
			t.Fatalf("escape must cancel, got %+v", r)
		}
	default:
		t.Fatal("expected reply")
	}
}

func TestPermissionDenyWithFeedback(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.AskReply, 1)
	o.beginPermissionAsk(controller.PermissionAskMsg{
		Request: permission.Request{Tool: "bash", Action: permission.ActionBash, Command: "curl https://x"},
		Reply:   reply,
	})
	o.acceptPermissionOption(askOptDenyFeedback)
	if o.perm == nil || !o.perm.feedbackMode {
		t.Fatal("expected feedback mode")
	}
	o.perm.feedback = "use docs instead"
	o.resolvePermission(controller.AskReply{Feedback: o.perm.feedback})
	r := <-reply
	if r.Approved || r.Feedback != "use docs instead" {
		t.Fatalf("got %+v", r)
	}
}

func TestPermissionDismissClearsOverlay(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.AskReply, 1)
	o.beginPermissionAsk(controller.PermissionAskMsg{
		Request: permission.Request{Tool: "bash", Action: permission.ActionBash, Command: "curl https://x"},
		Reply:   reply,
	})
	o.Apply(controller.PermissionDismissMsg{})
	if o.perm != nil {
		t.Fatal("overlay should clear without consuming reply")
	}
	select {
	case <-reply:
		t.Fatal("dismiss must not send on reply")
	default:
	}
}

func TestDrawPermissionAskReplacesComposerSlot(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.AskReply, 1)
	o.beginPermissionAsk(controller.PermissionAskMsg{
		Request: permission.Request{Action: permission.ActionBash, Tool: "bash", Command: "rm -f todo.list"},
		Reason:  "Matches built-in permissions rule",
		Reply:   reply,
	})
	surf := o.drawPermissionAsk(components.DrawContext{
		Max:    components.Size{Width: 60, Height: 12},
		Method: 0,
	}, 60, 12)
	if surf.Size.Width != 60 || surf.Size.Height != 12 {
		t.Fatalf("size=%v", surf.Size)
	}
}

func TestFormatAskHeader(t *testing.T) {
	h, d := formatAskHeader(permission.Request{Action: permission.ActionWrite, Paths: []string{"/tmp/a"}})
	if h != "Allow creating file:" || d != "/tmp/a" {
		t.Fatalf("%q %q", h, d)
	}
}

func TestContinueAskResolveContinue(t *testing.T) {
	activity := controller.NewActivityHandler(nil)
	o := testOverlays(activity)
	reply := make(chan controller.ContinueReply, 1)
	o.beginContinueAsk(controller.ContinueAskMsg{MaxRounds: 64, Reply: reply})
	if o.cont == nil {
		t.Fatal("expected continueAsk")
	}
	if o.cont.maxRounds != 64 {
		t.Fatalf("maxRounds=%d", o.cont.maxRounds)
	}
	if activity.Current != controller.ActivityAwaitingApproval {
		t.Fatalf("activity=%v", activity.Current)
	}
	o.resolveContinue(controller.ContinueReply{Continue: true})
	if o.cont != nil {
		t.Fatal("expected continueAsk cleared")
	}
	select {
	case r := <-reply:
		if !r.Continue {
			t.Fatal("expected Continue=true")
		}
	default:
		t.Fatal("expected reply")
	}
}

func TestContinueAskEscapeStops(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.ContinueReply, 1)
	o.beginContinueAsk(controller.ContinueAskMsg{MaxRounds: 2, Reply: reply})
	ctx := &components.EventContext{}
	_ = o.handleContinueKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
	select {
	case r := <-reply:
		if r.Continue {
			t.Fatal("escape should stop")
		}
	default:
		t.Fatal("expected reply on escape")
	}
}

func TestContinueDismissClearsOverlay(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.ContinueReply, 1)
	o.beginContinueAsk(controller.ContinueAskMsg{MaxRounds: 2, Reply: reply})
	o.Apply(controller.ContinueDismissMsg{})
	if o.cont != nil {
		t.Fatal("overlay should clear without consuming reply")
	}
	select {
	case <-reply:
		t.Fatal("dismiss must not send on reply")
	default:
	}
}

func TestBeginAskResolvesWhicheverAskWasShowing(t *testing.T) {
	activity := controller.NewActivityHandler(nil)
	o := testOverlays(activity)
	contReply := make(chan controller.ContinueReply, 1)
	o.beginContinueAsk(controller.ContinueAskMsg{MaxRounds: 4, Reply: contReply})

	permReply := make(chan controller.AskReply, 1)
	o.beginPermissionAsk(controller.PermissionAskMsg{
		Request: permission.Request{Tool: "bash", Action: permission.ActionBash, Command: "true"},
		Reply:   permReply,
	})
	if o.cont != nil {
		t.Fatal("a new ask must resolve the one already showing")
	}
	select {
	case r := <-contReply:
		if r.Continue {
			t.Fatal("displaced continue ask must resolve as stop")
		}
	default:
		t.Fatal("displaced continue ask must answer its channel")
	}
	if o.perm == nil || activity.Current != controller.ActivityAwaitingApproval {
		t.Fatalf("perm=%v activity=%v", o.perm, activity.Current)
	}
}

func TestPermissionAskHeightGrowsWithWrapping(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	long := "curl --silent --show-error --location --retry 3 --max-time 30 https://example.invalid/very/long/path?with=query&params=attached"
	o.beginPermissionAsk(controller.PermissionAskMsg{
		Request: permission.Request{Tool: "bash", Action: permission.ActionBash, Command: long},
		Reply:   make(chan controller.AskReply, 1),
	})

	wide := o.perm.preferredAskHeight(components.DefaultTheme(), 120, 0)
	narrow := o.perm.preferredAskHeight(components.DefaultTheme(), 34, 0)
	if narrow <= wide {
		t.Fatalf("narrow=%d must exceed wide=%d: wrapped detail needs more rows", narrow, wide)
	}

	// The estimate must fit every option row: paintAskPanel drops body rows
	// past height-2, which is what made late options unreachable.
	body := o.perm.askRows(components.DefaultTheme(), askInnerWidth(34), 0)
	if narrow < len(body)+2 {
		t.Fatalf("height=%d cannot fit %d rows + border", narrow, len(body))
	}
}

func TestQuestionHeightCountsRenderedRows(t *testing.T) {
	st := newQuestionAskState([]questiontool.Question{{
		Question: "ship it?",
		Header:   "h",
		Options: []questiontool.Option{
			{Label: "yes"},
			{Label: "no"},
		},
	}}, nil)

	th := components.DefaultTheme()
	// Two options without descriptions render one row each — the old
	// estimate doubled every option and over-allocated.
	got := st.preferredAskHeight(th, 80, 0)
	if want := max(len(st.askRows(th, askInnerWidth(80), 0))+2, 8); got != want {
		t.Fatalf("height=%d want=%d", got, want)
	}
	if got != 8 {
		t.Fatalf("two description-less options fit the minimum panel, got %d", got)
	}

	st.questions[0].Options[0].Label = "a label long enough to wrap on a narrow terminal several times over"
	if narrow := st.preferredAskHeight(th, 30, 0); narrow <= got {
		t.Fatalf("narrow=%d must exceed wide=%d: wrapped labels need more rows", narrow, got)
	}
}
