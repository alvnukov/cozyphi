package overlays

import (
	"fmt"
	"strings"
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
	for _, r := range "use docs instead now" {
		o.handlePermissionFeedbackKey(&components.EventContext{},
			xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: r})
	}
	// The field is a real editor, not an append-only buffer: word delete works.
	o.handlePermissionFeedbackKey(&components.EventContext{},
		xui.KeyEvent{Press: true, Code: xui.KeyBackspace, Mods: xui.ModCtrl})
	o.handlePermissionFeedbackKey(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	r := <-reply
	if r.Approved || r.Feedback != "use docs instead" {
		t.Fatalf("got %+v", r)
	}
}

func TestAskPasteLandsInFeedbackField(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.AskReply, 1)
	o.beginPermissionAsk(controller.PermissionAskMsg{
		Request: permission.Request{Tool: "bash", Action: permission.ActionBash, Command: "curl https://x"},
		Reply:   reply,
	})
	// No field is taking text yet, so the paste is not the overlay's to eat.
	if o.HandleAskPaste(&components.EventContext{}, xui.PasteEvent{Text: "docs/a.md"}) {
		t.Fatal("paste claimed before feedback mode")
	}
	o.acceptPermissionOption(askOptDenyFeedback)
	if !o.HandleAskPaste(&components.EventContext{}, xui.PasteEvent{Text: "see docs/a.md\nline two"}) {
		t.Fatal("paste not claimed in feedback mode")
	}
	// The newline flattens: the prompt is one row and has nowhere to put it.
	if got := o.perm.feedback.Value; got != "see docs/a.md line two" {
		t.Fatalf("pasted %q", got)
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
	body, _ := o.perm.askRows(components.DefaultTheme(), askInnerWidth(34), 0)
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

func lineText(l components.RichLine) string {
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.Text)
	}
	return b.String()
}

func bashAsk(t *testing.T) (*Overlays, chan controller.AskReply) {
	t.Helper()
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.AskReply, 1)
	o.beginPermissionAsk(controller.PermissionAskMsg{
		Request: permission.Request{Tool: "bash", Action: permission.ActionBash, Command: "curl https://x"},
		Reply:   reply,
	})
	return o, reply
}

func pressPerm(o *Overlays, r rune) {
	o.handlePermissionKey(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: r})
}

func TestPermissionAskDigitPicksOption(t *testing.T) {
	// A panel that prints "[2]" has to answer a bare 2 — the modifier the
	// options never mentioned was the reason digits looked broken.
	o, reply := bashAsk(t)
	pressPerm(o, '2')
	r := <-reply
	if !r.Approved || !r.AllowSession {
		t.Fatalf("digit 2 must allow the session, got %+v", r)
	}
}

func TestPermissionAskYesNoKeys(t *testing.T) {
	o, reply := bashAsk(t)
	pressPerm(o, 'y')
	if r := <-reply; !r.Approved || r.AllowSession || r.AllowPersistent {
		t.Fatalf("y must approve this call only, got %+v", r)
	}

	o, reply = bashAsk(t)
	pressPerm(o, 'n')
	if r := <-reply; r.Approved {
		t.Fatalf("n must deny, got %+v", r)
	}
	if o.perm != nil {
		t.Fatal("n closes the ask, like Esc")
	}
}

func TestPermissionAskSelectionWraps(t *testing.T) {
	// k on the first row used to do nothing at all, with nothing on screen
	// to say why; it now walks the options as a ring, like ↑.
	o, _ := bashAsk(t)
	pressPerm(o, 'k')
	if o.perm.ring.Selected() != len(askOptionLabels)-1 {
		t.Fatalf("selected=%d want %d", o.perm.ring.Selected(), len(askOptionLabels)-1)
	}
	pressPerm(o, 'j')
	if o.perm.ring.Selected() != 0 {
		t.Fatalf("selected=%d want 0", o.perm.ring.Selected())
	}
}

func TestPermissionAskHintsUnusableKey(t *testing.T) {
	o, reply := bashAsk(t)
	pressPerm(o, 'q')
	if o.perm == nil {
		t.Fatal("an unusable key must not answer the ask")
	}
	if o.perm.hint == "" {
		t.Fatal("a swallowed key must say why nothing happened")
	}
	select {
	case r := <-reply:
		t.Fatalf("unusable key must not reply, got %+v", r)
	default:
	}

	pressPerm(o, 'j')
	if o.perm.hint != "" {
		t.Fatalf("a key that works clears the hint, got %q", o.perm.hint)
	}
}

func TestFitAskBodyKeepsAnswerRows(t *testing.T) {
	body := make([]components.RichLine, 0, 20)
	for i := range 20 {
		body = append(body, components.RichLine{{Text: fmt.Sprintf("row %d", i)}})
	}

	const avail, answer = 8, 4
	got := fitAskBody(components.DefaultTheme(), body, avail, answer, 40, 0)
	if len(got) != avail {
		t.Fatalf("rows=%d want %d", len(got), avail)
	}
	// An ask nobody can answer stalls the run: detail goes, options stay.
	for i := range answer {
		want := lineText(body[len(body)-answer+i])
		if out := lineText(got[len(got)-answer+i]); out != want {
			t.Fatalf("answer row %d = %q want %q", i, out, want)
		}
	}
	if marker := lineText(got[len(got)-answer-1]); !strings.Contains(marker, "more lines") {
		t.Fatalf("elided body must say so, got %q", marker)
	}
}

func TestFormatAskHeaderListsEveryPath(t *testing.T) {
	// Three paths must not read as a request about one.
	h, d := formatAskHeader(permission.Request{
		Action: permission.ActionEdit,
		Paths:  []string{"/tmp/a", "/tmp/b"},
	})
	if h != "Allow editing files:" {
		t.Fatalf("header=%q", h)
	}
	if d != "/tmp/a\n/tmp/b" {
		t.Fatalf("detail=%q must list every path", d)
	}
}

func TestFormatAskHeaderClipsLongCommand(t *testing.T) {
	_, d := formatAskHeader(permission.Request{
		Action:  permission.ActionBash,
		Command: strings.Repeat("echo hi\n", 20),
	})
	lines := strings.Split(d, "\n")
	if len(lines) != askDetailLines+1 {
		t.Fatalf("lines=%d want %d", len(lines), askDetailLines+1)
	}
	if !strings.Contains(lines[len(lines)-1], "more lines") {
		t.Fatalf("clipped detail must read as clipped, got %q", lines[len(lines)-1])
	}
}

func TestContinueAskSharesPermissionKeys(t *testing.T) {
	press := func(o *Overlays, r rune) {
		o.handleContinueKey(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: r})
	}
	begin := func() (*Overlays, chan controller.ContinueReply) {
		o := testOverlays(controller.NewActivityHandler(nil))
		reply := make(chan controller.ContinueReply, 1)
		o.beginContinueAsk(controller.ContinueAskMsg{MaxRounds: 4, Reply: reply})
		return o, reply
	}

	o, reply := begin()
	press(o, '1')
	if r := <-reply; !r.Continue {
		t.Fatal("digit 1 must continue")
	}

	o, reply = begin()
	press(o, 'n')
	if r := <-reply; r.Continue {
		t.Fatal("n must stop")
	}

	// A digit past the last option is as unusable as any other key.
	o, reply = begin()
	press(o, '3')
	if o.cont == nil || o.cont.hint == "" {
		t.Fatal("an out-of-range digit must hint, not answer")
	}
	select {
	case r := <-reply:
		t.Fatalf("out-of-range digit must not reply, got %+v", r)
	default:
	}

	press(o, 'k')
	if o.cont.ring.Selected() != len(continueOptionLabels)-1 || o.cont.hint != "" {
		t.Fatalf("selected=%d hint=%q", o.cont.ring.Selected(), o.cont.hint)
	}
}

func TestPermissionAskSpaceActsLikeEnter(t *testing.T) {
	// Space takes the highlighted option in every list; here that means the
	// option j walked to, not a blanket approve.
	o, reply := bashAsk(t)
	pressPerm(o, 'j')
	pressPerm(o, ' ')
	r := <-reply
	if !r.Approved || !r.AllowSession {
		t.Fatalf("space must take the highlighted option, got %+v", r)
	}
}
