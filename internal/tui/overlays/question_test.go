package overlays

import (
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tools/questiontool"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

func TestQuestionAskBeginSetsState(t *testing.T) {
	activity := controller.NewActivityHandler(nil)
	o := testOverlays(activity)
	reply := make(chan controller.QuestionReply, 1)
	o.beginQuestionAsk(controller.QuestionAskMsg{
		Questions: []questiontool.Question{
			{Question: "q", Header: "h", Options: []questiontool.Option{{Label: "a", Description: "aa"}}},
		},
		Reply: reply,
	})
	if o.question == nil {
		t.Fatal("expected questionAsk state")
	}
	if len(o.question.questions) != 1 {
		t.Fatalf("questions=%d", len(o.question.questions))
	}
	if activity.Current != controller.ActivityAwaitingApproval {
		t.Fatalf("activity=%v", activity.Current)
	}
}

func TestQuestionResolveSendsReply(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.QuestionReply, 1)
	o.beginQuestionAsk(controller.QuestionAskMsg{
		Questions: []questiontool.Question{{Question: "q", Header: "h", Options: []questiontool.Option{{Label: "a"}}}},
		Reply:     reply,
	})
	o.resolveQuestion(controller.QuestionReply{Answers: []questiontool.Answer{{"a"}}})
	if o.question != nil {
		t.Fatal("expected cleared")
	}
	r := <-reply
	if len(r.Answers) != 1 || r.Answers[0][0] != "a" {
		t.Fatalf("answers=%v", r.Answers)
	}
}

func TestQuestionEscapeDismisses(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.QuestionReply, 1)
	o.beginQuestionAsk(controller.QuestionAskMsg{
		Questions: []questiontool.Question{{Question: "q", Header: "h", Options: []questiontool.Option{{Label: "a"}}}},
		Reply:     reply,
	})
	ctx := &components.EventContext{}
	if !o.handleQuestionKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEscape}) {
		t.Fatal("expected consume")
	}
	if o.question != nil {
		t.Fatal("expected overlay closed")
	}
	select {
	case r := <-reply:
		if len(r.Answers) != 0 {
			t.Fatalf("escape must send empty answers, got %v", r.Answers)
		}
	default:
		t.Fatal("expected reply")
	}
}

func TestQuestionSingleSelectEnterSubmitsFirstOption(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.QuestionReply, 1)
	o.beginQuestionAsk(controller.QuestionAskMsg{
		Questions: []questiontool.Question{
			{
				Question: "q",
				Header:   "h",
				Options: []questiontool.Option{
					{Label: "Build", Description: "go"},
					{Label: "Plan", Description: "think"},
				},
			},
		},
		Reply: reply,
	})
	ctx := &components.EventContext{}
	// selected defaults to 0; enter picks the first option and submits (single question).
	if !o.handleQuestionKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter}) {
		t.Fatal("expected consume")
	}
	r := <-reply
	if len(r.Answers) != 1 || r.Answers[0][0] != "Build" {
		t.Fatalf("answers=%v", r.Answers)
	}
}

func TestQuestionMultiToggle(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.QuestionReply, 1)
	o.beginQuestionAsk(controller.QuestionAskMsg{
		Questions: []questiontool.Question{
			{Question: "q", Header: "h", Multiple: true, Options: []questiontool.Option{{Label: "A"}, {Label: "B"}}},
		},
		Reply: reply,
	})
	ctx := &components.EventContext{}
	o.handleQuestionKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	if !contains(o.question.answers[0], "A") {
		t.Fatal("A should be picked")
	}
	o.handleQuestionKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyDown})
	o.handleQuestionKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	if !contains(o.question.answers[0], "B") {
		t.Fatal("B should be picked")
	}
}

func TestQuestionDismissClearsOverlay(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.QuestionReply, 1)
	o.beginQuestionAsk(controller.QuestionAskMsg{
		Questions: []questiontool.Question{{Question: "q", Header: "h", Options: []questiontool.Option{{Label: "a"}}}},
		Reply:     reply,
	})
	o.Apply(controller.QuestionDismissMsg{})
	if o.question != nil {
		t.Fatal("overlay should clear without consuming reply")
	}
	select {
	case <-reply:
		t.Fatal("dismiss must not send on reply")
	default:
	}
}
