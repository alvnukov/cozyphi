package app

import (
	"fmt"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

// tooltipDwell is how long the pointer must rest on one control before its
// hint appears: long enough that sweeping the pointer across the screen
// stays silent, short enough to feel like an answer rather than a delay.
const tooltipDwell = 500 * time.Millisecond

// tooltipEngine tracks one hint. It owns no timer and no goroutine: pointer
// motion arms it, the scheduler wake at the deadline reveals it, and any
// other input or region change drops it. A frame is requested only when the
// hint appears or disappears, so a resting pointer costs nothing.
type tooltipEngine struct {
	// region names the hovered control the hint is about.
	region string
	// deadline is when an armed hint may become visible.
	deadline time.Time
	armed    bool
	// visible is set once the dwell has passed and the text resolved.
	visible bool
	// text and at capture the hint content where it was revealed.
	text string
	at   components.Point
	// suppress names a control whose hint a click dismissed; it stays quiet
	// until the pointer dwells on a different control first.
	suppress string
}

// dwell reacts to pointer motion: resting on a control re-arms the hint
// (a traveling pointer must not pop hints), moving to another control
// swaps it, and leaving every control drops it. A suppressed control stays
// quiet.
func (t *tooltipEngine) dwell(region string, now time.Time) {
	if region == "" || region == t.suppress {
		t.hide()
		return
	}
	// Dwelling elsewhere clears the suppression: coming back to the
	// dismissed control later shows its hint again, refreshed.
	t.suppress = ""
	if t.region != region {
		t.hide()
		t.region = region
	}
	if t.visible {
		return
	}
	t.armed = true
	t.deadline = now.Add(tooltipDwell)
}

// hide drops the hint without remembering the control.
func (t *tooltipEngine) hide() {
	t.armed = false
	t.visible = false
	t.region = ""
	t.text = ""
}

// dismiss drops a visible hint and quiets its control until the pointer
// dwells on something else.
func (t *tooltipEngine) dismiss() {
	if t.region != "" {
		t.suppress = t.region
	}
	t.hide()
}

// reconcile drops hints whose control is no longer hovered: relayouts move
// regions under a still pointer (refreshHover resolves hover, not dwell).
func (t *tooltipEngine) reconcile(region string) {
	if (t.armed || t.visible) && region != t.region {
		t.hide()
	}
}

// due reports whether an armed dwell has elapsed and the hint may appear.
func (t *tooltipEngine) due(now time.Time) bool {
	return t.armed && !t.visible && !now.Before(t.deadline)
}

// wake is the instant the engine wants the next frame: the reveal.
func (t *tooltipEngine) wake() time.Time {
	if t.armed && !t.visible {
		return t.deadline
	}
	return time.Time{}
}

// show publishes the resolved hint; an empty answer cancels the dwell
// instead of floating an empty panel.
func (t *tooltipEngine) show(text string, at components.Point) {
	if text == "" {
		t.hide()
		return
	}
	t.armed = false
	t.visible = true
	t.text = text
	t.at = at
}

// trackTooltip turns one mouse event into hint input: motion re-arms the
// dwell on the control under the pointer; clicks and wheel steps dismiss
// the hint — the screen is about to change under it. A frame is requested
// only when visibility flips; motion inside a control stays free.
func (a *App) trackTooltip(e xui.MouseEvent) {
	was := a.tooltip.visible
	if e.Action == xui.MouseMotion && e.Button == xui.MouseNone && e.Wheel == 0 {
		a.tooltip.dwell(a.tooltipRegion(), time.Now())
	} else {
		a.tooltip.dismiss()
	}
	if a.tooltip.visible != was {
		a.redraw = true
	}
}

// hideTooltip drops a visible hint after a key press and repaints if it was
// on screen. The control is not suppressed: the key was not about it.
func (a *App) hideTooltip() {
	if a.tooltip.visible {
		a.tooltip.hide()
		a.redraw = true
	}
}

// tooltipRegion names the hovered control for the dwell: the widget plus
// its interior region, so two controls in one widget dwell separately.
// Only widgets that can answer a hint arm one.
func (a *App) tooltipRegion() string {
	// A root that seized the pointer — a modal ask — hides whatever the hit
	// test would name: no dwell hint may float over its panel.
	if owner, ok := a.root.(components.PointerOwner); ok && owner.OwnsPointer() {
		return ""
	}
	h := a.hover
	if h == nil {
		return ""
	}
	if _, ok := h.Widget.(components.HoverTooltiper); !ok {
		return ""
	}
	key := fmt.Sprintf("%p", h.Widget)
	if regions, ok := h.Widget.(components.HoverRegioner); ok {
		key = fmt.Sprintf("%s|%d", key, regions.HoverRegion(h.X, h.Y))
	}
	return key
}

// hoverHint resolves the hint text for the hovered control.
func hoverHint(h *components.HoverState) string {
	if h == nil {
		return ""
	}
	if tipper, ok := h.Widget.(components.HoverTooltiper); ok {
		if text, ok := tipper.HoverTooltip(h.X, h.Y); ok && text != "" {
			return text
		}
	}
	return ""
}
