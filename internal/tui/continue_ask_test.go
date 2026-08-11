package tui

import (
	"testing"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/xui"
)

func TestContinueAskResolveContinue(t *testing.T) {
	editor := &Editor{theme: components.DefaultTheme(), activity: NewActivityHandler(nil)}
	reply := make(chan ContinueReply, 1)
	editor.beginContinueAsk(ContinueAskMsg{MaxRounds: 64, Reply: reply})
	if editor.continueAsk == nil {
		t.Fatal("expected continueAsk")
	}
	if editor.continueAsk.maxRounds != 64 {
		t.Fatalf("maxRounds=%d", editor.continueAsk.maxRounds)
	}
	if editor.activity.Current != ActivityAwaitingApproval {
		t.Fatalf("activity=%v", editor.activity.Current)
	}
	editor.resolveContinue(ContinueReply{Continue: true})
	if editor.continueAsk != nil {
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
	editor := &Editor{theme: components.DefaultTheme(), activity: NewActivityHandler(nil)}
	reply := make(chan ContinueReply, 1)
	editor.beginContinueAsk(ContinueAskMsg{MaxRounds: 2, Reply: reply})
	ctx := &components.EventContext{}
	_ = editor.handleContinueKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
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
	editor := &Editor{theme: components.DefaultTheme(), activity: NewActivityHandler(nil)}
	reply := make(chan ContinueReply, 1)
	editor.beginContinueAsk(ContinueAskMsg{MaxRounds: 2, Reply: reply})
	editor.Update(ContinueDismissMsg{})
	if editor.continueAsk != nil {
		t.Fatal("overlay should clear without consuming reply")
	}
	select {
	case <-reply:
		t.Fatal("dismiss must not send on reply")
	default:
	}
}
