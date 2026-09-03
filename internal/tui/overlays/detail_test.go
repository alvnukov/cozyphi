package overlays

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tui/browse"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

// longCommand is fifty numbered lines — long enough that the collapsed
// window cannot show it and every line is distinguishable when scrolling.
func longCommand() string {
	const n = 50
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %02d\n", i)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func beginLongCommandAsk(o *Overlays) chan controller.AskReply {
	reply := make(chan controller.AskReply, 1)
	o.beginPermissionAsk(controller.PermissionAskMsg{
		Request: permission.Request{Tool: "bash", Action: permission.ActionBash, Command: longCommand()},
		Reply:   reply,
	})
	return reply
}

func pressAsk(o *Overlays, code xui.KeyCode, r rune) {
	o.handlePermissionKey(&components.EventContext{}, xui.KeyEvent{Press: true, Code: code, Rune: r})
}

func detailText(st *permAskState, th components.Theme) string {
	rows := st.detailLines(th, askInnerWidth(80), 0)
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		parts = append(parts, lineText(row))
	}
	return strings.Join(parts, "\n")
}

// TestAskDetailExpandReachesEveryLine: a fifty-line command is readable
// end to end inside the dialog — v opens the detail, the motions walk the
// window to the last line, and the markers count what is off screen.
func TestAskDetailExpandReachesEveryLine(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	beginLongCommandAsk(o)
	st := o.perm
	th := components.DefaultTheme()

	if got := detailText(st, th); strings.Contains(got, "line 50") {
		t.Fatal("the collapsed window must clip the tail")
	}
	pressAsk(o, xui.KeyRune, 'v')
	if !st.expanded {
		t.Fatal("v must expand the detail")
	}
	for range 60 {
		pressAsk(o, xui.KeyDown, 0)
	}
	got := detailText(st, th)
	if !strings.Contains(got, "line 50") {
		t.Fatalf("scrolling down must reach the last line, got:\n%s", got)
	}
	if !strings.Contains(got, "lines above") || strings.Contains(got, "lines below") {
		t.Fatalf("at the bottom only the above marker remains, got:\n%s", got)
	}
	for range 100 {
		pressAsk(o, xui.KeyRune, 'k')
	}
	got = detailText(st, th)
	if !strings.Contains(got, "line 01") || strings.Contains(got, "lines above") {
		t.Fatalf("scrolling back up must land on the first line, got:\n%s", got)
	}
}

// TestAskDetailEscBacksOutOneLevel: the first Esc folds the detail, only
// the second denies — Esc never jumps two levels at once.
func TestAskDetailEscBacksOutOneLevel(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := beginLongCommandAsk(o)
	pressAsk(o, xui.KeyRune, 'v')

	pressAsk(o, xui.KeyEscape, 0)
	if o.perm == nil || o.perm.expanded {
		t.Fatal("the first Esc must collapse the detail and keep the ask")
	}
	select {
	case <-reply:
		t.Fatal("collapsing must not answer the ask")
	default:
	}

	pressAsk(o, xui.KeyEscape, 0)
	if o.perm != nil {
		t.Fatal("the second Esc must deny")
	}
	select {
	case r := <-reply:
		if r.Approved {
			t.Fatalf("got %+v", r)
		}
	default:
		t.Fatal("expected a denial reply")
	}
}

// TestAskDetailDigitsStillAnswerWhileExpanded: reading never blocks
// deciding — a digit answers with the detail open.
func TestAskDetailDigitsStillAnswerWhileExpanded(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := beginLongCommandAsk(o)
	pressAsk(o, xui.KeyRune, 'v')

	pressAsk(o, xui.KeyRune, '2')
	select {
	case r := <-reply:
		if !r.Approved || !r.AllowSession {
			t.Fatalf("got %+v", r)
		}
	default:
		t.Fatal("expected the digit to answer while expanded")
	}
}

// TestAskDetailWheelScrollsWhileExpanded: the expanded detail is a scroll
// surface — the wheel moves its window, not the option ring.
func TestAskDetailWheelScrollsWhileExpanded(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	beginLongCommandAsk(o)
	drawTestPanel(t, o)
	pressAsk(o, xui.KeyRune, 'v')

	wheel := xui.MouseEvent{X: 40, Y: 12, Button: xui.MouseWheelDown, Action: xui.MousePress}
	o.HandleAskMouse(&components.EventContext{}, wheel)
	if o.perm.detailScroll != browse.WheelStep {
		t.Fatalf("detailScroll=%d want %d", o.perm.detailScroll, browse.WheelStep)
	}
	if o.perm.ring.Selected() != 0 {
		t.Fatal("the wheel must not step the ring while the detail is open")
	}
}

// TestAskShortPanelNeverHidesTheOptions: on a terminal too short for the
// detail, the detail is what gives way — every option and the hint stay
// on screen, because an ask nobody can answer stalls the run.
func TestAskShortPanelNeverHidesTheOptions(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	beginLongCommandAsk(o)

	surf := o.drawPermissionAsk(components.DrawContext{}, 80, 8)
	text := components.SurfaceText(surf)
	for _, label := range askOptionLabels {
		if !strings.Contains(text, label) {
			t.Fatalf("option %q must survive a short panel, got:\n%s", label, text)
		}
	}
}

// TestAskEditEvidenceShowsTheDiff: an edit ask that carries a preview
// shows the change itself, styled as a diff, with the path list above it.
func TestAskEditEvidenceShowsTheDiff(t *testing.T) {
	th := components.DefaultTheme()
	st := newPermAskState(controller.PermissionAskMsg{Request: permission.Request{
		Tool:    "edit",
		Action:  permission.ActionEdit,
		Paths:   []string{"/tmp/a.go"},
		Preview: "--- a/a.go\n+++ b/a.go\n@@ -1,2 +1,2 @@\n-old line\n+new line",
	}, Reply: make(chan controller.AskReply, 1)})

	rows := st.detailRows(th, askInnerWidth(80), 0)
	var added, removed components.RichLine
	for _, row := range rows {
		switch lineText(row) {
		case "+new line":
			added = row
		case "-old line":
			removed = row
		}
	}
	if added == nil || removed == nil {
		t.Fatalf("the diff rows must render, got %q", detailText(st, th))
	}
	if added[0].Style != th.Success || removed[0].Style != th.Destructive {
		t.Fatal("added rows wear Success, removed rows wear Destructive")
	}
	if !strings.Contains(detailText(st, th), "/tmp/a.go") {
		t.Fatal("the path list stays above the diff")
	}
}

// TestAskEditWithoutPreviewFallsBackToPaths: no preview, no diff — the
// ask shows what it always showed.
func TestAskEditWithoutPreviewFallsBackToPaths(t *testing.T) {
	st := newPermAskState(controller.PermissionAskMsg{Request: permission.Request{
		Tool:   "edit",
		Action: permission.ActionEdit,
		Paths:  []string{"/tmp/a.go", "/tmp/b.go"},
	}, Reply: make(chan controller.AskReply, 1)})
	if st.detail != "/tmp/a.go\n/tmp/b.go" {
		t.Fatalf("detail=%q", st.detail)
	}
}
