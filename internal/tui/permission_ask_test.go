package tui

import (
	"testing"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/permission"
	"github.com/pulseaiclub/phi/internal/tui/controller"
)

func TestResolvePermissionSendsReply(t *testing.T) {
	editor := &Editor{theme: components.DefaultTheme(), activity: controller.NewActivityHandler(nil)}
	reply := make(chan controller.AskReply, 1)
	editor.beginPermissionAsk(controller.PermissionAskMsg{
		Request: permission.Request{Action: permission.ActionBash, Tool: "bash", Command: "curl x"},
		Reason:  "needs approval",
		Reply:   reply,
	})
	if editor.permAsk == nil {
		t.Fatal("expected permAsk")
	}
	if editor.permAsk.header != "Run this command?" {
		t.Fatalf("header=%q", editor.permAsk.header)
	}
	if editor.activity.Current != controller.ActivityAwaitingApproval {
		t.Fatalf("activity=%v", editor.activity.Current)
	}
	editor.resolvePermission(controller.AskReply{Approved: true})
	if editor.permAsk != nil {
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

func TestPermissionDenyWithFeedback(t *testing.T) {
	editor := &Editor{theme: components.DefaultTheme(), activity: controller.NewActivityHandler(nil)}
	reply := make(chan controller.AskReply, 1)
	editor.beginPermissionAsk(controller.PermissionAskMsg{
		Request: permission.Request{Tool: "bash", Action: permission.ActionBash, Command: "curl https://x"},
		Reply:   reply,
	})
	editor.acceptPermissionOption(askOptDenyFeedback)
	if editor.permAsk == nil || !editor.permAsk.feedbackMode {
		t.Fatal("expected feedback mode")
	}
	editor.permAsk.feedback = "use docs instead"
	editor.resolvePermission(controller.AskReply{Feedback: editor.permAsk.feedback})
	r := <-reply
	if r.Approved || r.Feedback != "use docs instead" {
		t.Fatalf("got %+v", r)
	}
}

func TestPermissionDismissClearsOverlay(t *testing.T) {
	editor := &Editor{theme: components.DefaultTheme(), activity: controller.NewActivityHandler(nil)}
	reply := make(chan controller.AskReply, 1)
	editor.beginPermissionAsk(controller.PermissionAskMsg{
		Request: permission.Request{Tool: "bash", Action: permission.ActionBash, Command: "curl https://x"},
		Reply:   reply,
	})
	editor.Update(controller.PermissionDismissMsg{})
	if editor.permAsk != nil {
		t.Fatal("overlay should clear without consuming reply")
	}
	select {
	case <-reply:
		t.Fatal("dismiss must not send on reply")
	default:
	}
}

func TestDrawPermissionAskReplacesComposerSlot(t *testing.T) {
	editor := &Editor{theme: components.DefaultTheme(), activity: controller.NewActivityHandler(nil)}
	reply := make(chan controller.AskReply, 1)
	editor.beginPermissionAsk(controller.PermissionAskMsg{
		Request: permission.Request{Action: permission.ActionBash, Tool: "bash", Command: "rm -f todo.list"},
		Reason:  "Matches built-in permissions rule",
		Reply:   reply,
	})
	surf := editor.drawPermissionAsk(components.DrawContext{
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
