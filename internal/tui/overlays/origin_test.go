package overlays

import (
	"strings"
	"testing"

	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tools/questiontool"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

func childOrigin() AskOrigin {
	return AskOrigin{Owner: "job-1", Label: "explore(read the loader)"}
}

func childBashAsk(reply chan controller.AskReply) controller.PermissionAskMsg {
	return controller.PermissionAskMsg{
		Request:     permission.Request{Tool: "bash", Action: permission.ActionBash, Command: "curl https://x"},
		Reply:       reply,
		PersistPath: "/home/u/.cozyphi/config.yaml",
	}
}

// TestRoutedAskNamesTheSessionItCameFrom: an ask shown on another
// session's screen says whose call it is, before the header.
func TestRoutedAskNamesTheSessionItCameFrom(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	o.ApplyFrom(childBashAsk(make(chan controller.AskReply, 1)), childOrigin())
	got := askBodyText(o)
	if !strings.HasPrefix(got, "[explore(read the loader)] Run this command?") {
		t.Fatalf("the header must wear the origin, got:\n%s", got)
	}

	o.resolvePermission(controller.AskReply{})
	o.ApplyFrom(childBashAsk(make(chan controller.AskReply, 1)), AskOrigin{Owner: "job-1"})
	if got := askBodyText(o); !strings.HasPrefix(got, "Run this command?") {
		t.Fatalf("a session looking at its own ask needs no label, got:\n%s", got)
	}
}

// TestChildAskDropsThePermanentGrant: the rule that outlives every session
// is not offered for a call the user is answering on a sub-agent's behalf;
// the session-wide grant, which binds to that sub-agent, stays.
func TestChildAskDropsThePermanentGrant(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.AskReply, 1)
	o.ApplyFrom(childBashAsk(reply), childOrigin())

	got := askBodyText(o)
	if strings.Contains(got, "Every Session") {
		t.Fatalf("a child's ask must not offer the permanent grant, got:\n%s", got)
	}
	if !strings.Contains(got, "Allow All for This Session [2]") {
		t.Fatalf("the session grant keeps its place, got:\n%s", got)
	}
	if !strings.Contains(got, "Deny with feedback [3]") {
		t.Fatalf("the options renumber to what is drawn, got:\n%s", got)
	}
	if !strings.Contains(got, "1-3 or y/n") {
		t.Fatalf("the hint counts the options offered, got:\n%s", got)
	}

	// 3 is the last option now, and it is deny-with-feedback, not a grant.
	o.acceptPermissionOption(mustOption(t, o, 2))
	if o.perm == nil || !o.perm.feedbackMode {
		t.Fatal("option 3 of a child's ask is deny with feedback")
	}
	select {
	case r := <-reply:
		t.Fatalf("nothing is granted by opening the feedback prompt, got %+v", r)
	default:
	}
}

func mustOption(t *testing.T, o *Overlays, idx int) askOption {
	t.Helper()
	opt, ok := o.perm.option(idx)
	if !ok {
		t.Fatalf("option %d must be drawn", idx)
	}
	return opt
}

// TestDismissOnlyLandsOnTheAskItsOwnerStarted: two sessions share one
// overlay, so a dismissal from the wrong one must leave the ask standing.
func TestDismissOnlyLandsOnTheAskItsOwnerStarted(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	o.ApplyFrom(childBashAsk(make(chan controller.AskReply, 1)), childOrigin())

	o.Apply(controller.PermissionDismissMsg{})
	if o.perm == nil {
		t.Fatal("the host's own dismissal must not close a child's ask")
	}
	o.ApplyFrom(controller.PermissionDismissMsg{}, AskOrigin{Owner: "job-2"})
	if o.perm == nil {
		t.Fatal("a sibling's dismissal must not close another child's ask")
	}
	o.ApplyFrom(controller.PermissionDismissMsg{}, childOrigin())
	if o.perm != nil {
		t.Fatal("the owner's dismissal closes it")
	}
}

// TestDenyFromAnswersOneSessionsAsks: a sub-agent that goes away takes its
// unanswered questions with it — denied, never granted.
func TestDenyFromAnswersOneSessionsAsks(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	perm := make(chan controller.AskReply, 1)
	o.ApplyFrom(childBashAsk(perm), childOrigin())

	if o.DenyFrom("job-2") {
		t.Fatal("another session's exit must not answer this ask")
	}
	if !o.DenyFrom("job-1") {
		t.Fatal("the owner's exit answers it")
	}
	if o.perm != nil {
		t.Fatal("expected the panel closed")
	}
	select {
	case r := <-perm:
		if r.Approved || r.AllowSession || r.AllowPersistent {
			t.Fatalf("a released child's ask is denied, got %+v", r)
		}
	default:
		t.Fatal("expected a reply on the child's own channel")
	}

	question := make(chan controller.QuestionReply, 1)
	o.ApplyFrom(controller.QuestionAskMsg{
		Questions: []questiontool.Question{{Header: "h", Question: "ship it?"}},
		Reply:     question,
	}, childOrigin())
	if !strings.Contains(askQuestionText(o), "[explore(read the loader)]") {
		t.Fatalf("a routed question names its session too, got:\n%s", askQuestionText(o))
	}
	if !o.DenyFrom("job-1") || o.question != nil {
		t.Fatal("a released child's question closes with it")
	}
	if _, ok := <-question; !ok {
		t.Fatal("expected a reply on the child's own channel")
	}
}

func askQuestionText(o *Overlays) string {
	body, _ := o.question.askRows(o.theme, askInnerWidth(80), 0)
	parts := make([]string, 0, len(body))
	for _, row := range body {
		parts = append(parts, lineText(row))
	}
	return strings.Join(parts, "\n")
}

// TestRoutedContinueAskNamesItsSession: the max-rounds ask travels with the
// same label as the permission ask.
func TestRoutedContinueAskNamesItsSession(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.ContinueReply, 1)
	o.ApplyFrom(controller.ContinueAskMsg{MaxRounds: 40, Reply: reply}, childOrigin())
	body, _ := o.cont.askRows(o.theme, askInnerWidth(80), 0)
	if got := lineText(body[0]); !strings.HasPrefix(got, "[explore(read the loader)] Reached max tool rounds") {
		t.Fatalf("the continue ask must wear the origin, got %q", got)
	}
}
