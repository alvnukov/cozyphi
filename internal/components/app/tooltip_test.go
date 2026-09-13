package app

import (
	"testing"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

// tipStub is a widget whose cells take the hand pointer, split into two
// regions, and can explain themselves — the minimum the dwell needs.
type tipStub struct {
	shape string
	tip   string
	miss  bool
}

func (*tipStub) Handle(*components.EventContext, xui.Event) {}

func (s *tipStub) PointerShape(_, _ int) string { return s.shape }

func (*tipStub) HoverRegion(x, _ int) int { return x / 5 }

func (s *tipStub) HoverTooltip(_, _ int) (string, bool) {
	if s.miss {
		return "", false
	}
	return s.tip, true
}

func (s *tipStub) Draw(components.DrawContext) components.Surface {
	return components.Surface{Size: components.Size{Width: 10, Height: 4}, Widget: s}
}

func motionAt(x, y int) xui.MouseEvent {
	return xui.MouseEvent{X: x, Y: y, Button: xui.MouseNone, Action: xui.MouseMotion}
}

func pressAt(x, y int) xui.MouseEvent {
	return xui.MouseEvent{X: x, Y: y, Button: xui.MouseLeft, Action: xui.MousePress}
}

// newTipApp builds an App whose painted frame holds one hinted control at
// (0,0)-(9,3), split into regions by the first local column pair.
func newTipApp(t *testing.T) (*App, *tipStub) {
	t.Helper()
	stub := &tipStub{shape: components.ShapePointer, tip: "Run the command"}
	root := (&plainStub{}).Draw(components.DrawContext{})
	root.Children = []components.SubSurface{{Origin: components.Point{}, Surface: stub.Draw(components.DrawContext{})}}
	a := &App{lastSurf: root}
	return a, stub
}

// Resting arms the hint at a fixed dwell ahead; continued motion inside the
// region re-arms, so a traveling pointer never pops a hint; the wake asks
// for exactly the reveal frame.
func TestTooltipDwellArmsAndRearms(t *testing.T) {
	var e tooltipEngine
	now := time.Unix(1000, 0)
	e.dwell("w|0", now)
	if !e.armed || e.visible {
		t.Fatalf("dwell must arm, not reveal: %+v", e)
	}
	if !e.deadline.Equal(now.Add(tooltipDwell)) {
		t.Fatalf("deadline = %v, want %v", e.deadline, now.Add(tooltipDwell))
	}
	if !e.wake().Equal(e.deadline) {
		t.Fatalf("wake must be the reveal instant")
	}

	later := now.Add(tooltipDwell - 100*time.Millisecond)
	e.dwell("w|0", later)
	if !e.deadline.Equal(later.Add(tooltipDwell)) {
		t.Fatalf("re-arm deadline = %v, want %v", e.deadline, later.Add(tooltipDwell))
	}
	if e.due(later.Add(tooltipDwell - time.Millisecond)) {
		t.Fatalf("hint must not be due before its deadline")
	}
	if !e.due(later.Add(tooltipDwell)) {
		t.Fatalf("hint must be due at its deadline")
	}
	e.show("hint", components.Point{X: 3, Y: 4})
	if !e.visible || e.armed || e.text != "hint" || e.at != (components.Point{X: 3, Y: 4}) {
		t.Fatalf("show must publish and disarm: %+v", e)
	}
	if !e.wake().IsZero() {
		t.Fatalf("a visible hint needs no wake")
	}
}

// Moving to another control swaps the hint; leaving every control drops it.
func TestTooltipDwellSwapsAndDrops(t *testing.T) {
	var e tooltipEngine
	now := time.Unix(1000, 0)
	e.dwell("w|0", now)
	e.dwell("w|1", now)
	if e.region != "w|1" || !e.armed {
		t.Fatalf("region swap must re-arm the new control: %+v", e)
	}
	e.dwell("", now)
	if e.armed || e.visible || e.region != "" {
		t.Fatalf("leaving every control must drop the hint: %+v", e)
	}
}

// A click dismisses the hint and quiets its control until the pointer
// dwells on a different one first.
func TestTooltipDismissSuppressesUntilAnotherControl(t *testing.T) {
	var e tooltipEngine
	now := time.Unix(1000, 0)
	e.dwell("w|0", now)
	e.show("hint", components.Point{})
	e.dismiss()
	if e.visible || e.armed || e.suppress != "w|0" {
		t.Fatalf("dismiss must hide and suppress: %+v", e)
	}
	e.dwell("w|0", now)
	if e.armed {
		t.Fatalf("the dismissed control must stay quiet")
	}
	e.dwell("w|1", now)
	if !e.armed || e.suppress != "" {
		t.Fatalf("dwelling elsewhere clears the suppression: %+v", e)
	}
	e.dwell("w|0", now)
	if !e.armed {
		t.Fatalf("returning later shows the hint again")
	}
}

// Relayouts move regions without mouse motion; a hint whose control is no
// longer hovered is dropped, armed or visible.
func TestTooltipReconcileDropsMovedControls(t *testing.T) {
	var e tooltipEngine
	now := time.Unix(1000, 0)
	e.dwell("w|0", now)
	e.reconcile("w|1")
	if e.armed || e.visible {
		t.Fatalf("a moved control must drop the armed hint: %+v", e)
	}
	e.dwell("w|0", now)
	e.show("hint", components.Point{})
	e.reconcile("w|0")
	if !e.visible {
		t.Fatalf("the same control keeps the visible hint")
	}
	e.reconcile("")
	if e.visible {
		t.Fatalf("a vanished control must drop the visible hint")
	}
}

// An empty answer cancels the dwell instead of floating an empty panel.
func TestTooltipShowEmptyCancels(t *testing.T) {
	var e tooltipEngine
	e.dwell("w|0", time.Unix(1000, 0))
	e.show("", components.Point{})
	if e.visible || e.armed {
		t.Fatalf("empty text must cancel: %+v", e)
	}
}

// Motion over a control that can explain itself arms the dwell; a press
// dismisses a visible hint. Motion over a plain control does nothing.
func TestTrackTooltipArmsMotionAndDismissesPress(t *testing.T) {
	a, _ := newTipApp(t)
	a.updateHover(1, 0)
	a.redraw = false // the hover change already asked for its own frame
	a.trackTooltip(motionAt(1, 0))
	if !a.tooltip.armed || a.tooltip.visible {
		t.Fatalf("motion must arm, not reveal: %+v", a.tooltip)
	}
	if a.redraw {
		t.Fatalf("arming must not request a frame")
	}
	a.tooltip.show("hint", components.Point{X: 1, Y: 0})
	a.trackTooltip(pressAt(1, 0))
	if a.tooltip.visible || a.tooltip.suppress == "" {
		t.Fatalf("press must dismiss and suppress: %+v", a.tooltip)
	}
	if !a.redraw {
		t.Fatalf("a disappearing hint repaints")
	}

	plain := &App{lastSurf: (&plainStub{}).Draw(components.DrawContext{})}
	plain.updateHover(5, 5)
	plain.trackTooltip(motionAt(5, 5))
	if plain.tooltip.armed {
		t.Fatalf("a control without a hint never arms")
	}
}

// The reveal resolves the text through the hovered widget and anchors at
// the pointer; a control that moved away from under the pointer drops it.
func TestRevealTooltipPublishesAndDrops(t *testing.T) {
	a, stub := newTipApp(t)
	a.updateHover(1, 0)
	a.trackTooltip(motionAt(1, 0))
	a.revealTooltip(time.Now().Add(tooltipDwell))
	if !a.tooltip.visible || a.tooltip.text != stub.tip {
		t.Fatalf("reveal must publish the widget's hint: %+v", a.tooltip)
	}
	if a.tooltip.at != (components.Point{X: 1, Y: 0}) {
		t.Fatalf("hint anchors at the pointer: %v", a.tooltip.at)
	}

	// Relayout moved the control away from under the still pointer.
	a.updateHover(50, 50)
	a.revealTooltip(time.Now().Add(tooltipDwell))
	if a.tooltip.visible {
		t.Fatalf("a vanished control drops the visible hint")
	}

	// A widget that answers nothing stays silent.
	quiet := &tipStub{shape: components.ShapePointer, miss: true}
	root := (&plainStub{}).Draw(components.DrawContext{})
	root.Children = []components.SubSurface{{Origin: components.Point{}, Surface: quiet.Draw(components.DrawContext{})}}
	b := &App{lastSurf: root}
	b.updateHover(1, 0)
	b.trackTooltip(motionAt(1, 0))
	b.revealTooltip(time.Now().Add(tooltipDwell))
	if b.tooltip.visible || b.tooltip.armed {
		t.Fatalf("an empty answer cancels the dwell: %+v", b.tooltip)
	}
}

// Typing hides a visible hint without suppressing its control: the key was
// not about it, and the pointer may still be resting there.
func TestHideTooltipOnKey(t *testing.T) {
	a, _ := newTipApp(t)
	a.updateHover(1, 0)
	a.trackTooltip(motionAt(1, 0))
	a.tooltip.show("hint", components.Point{X: 1, Y: 0})
	a.hideTooltip()
	if a.tooltip.visible || !a.redraw {
		t.Fatalf("a key hides the visible hint and repaints")
	}
	if a.tooltip.suppress != "" {
		t.Fatalf("a key does not suppress the control")
	}
}
