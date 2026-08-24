package overlays

import (
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/permission"
	"github.com/pulseaiclub/phi/internal/tui/controller"
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
