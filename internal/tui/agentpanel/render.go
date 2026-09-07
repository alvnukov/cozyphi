package agentpanel

import (
	"strconv"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

// actionGap separates the child's name from what it is doing: two spaces, not
// a "·", because the action is a second column of the row rather than another
// fact appended to the first.
const actionGap = "  "

// Draw paints the band: the key-hint row while it holds the keyboard, an
// optional "↑ N more" indicator, up to three list rows, an optional "↓ N more"
// indicator, and the pending notice over the last of them. It also asks the
// scheduler for the frames the band needs — one a second while a child runs,
// one when a window expires — because a widget never runs a timer of its own.
func (p *Panel) Draw(ctx components.DrawContext, width int) components.Surface {
	v := p.build()
	p.schedule(ctx, v)
	h := v.height()
	if width <= 0 || h == 0 {
		return components.NewSurface(max(width, 0), 0, nil)
	}

	th := p.palette()
	s := components.NewSurface(width, h, nil)
	y := 0
	if v.hint {
		// The same words the footer carries for this scope, one row above the
		// band, where the user's eye already is while they move through it.
		s.Print(0, y, padTo(keys.Hints(keys.ScopeAgents), width, ctx.Method), th.Muted, ctx.Method)
		y++
	}
	if v.above > 0 {
		s.Print(0, y, indicator(glyphUp, v.above, width, ctx.Method), th.Muted, ctx.Method)
		y++
	}
	for i := range v.visible {
		p.drawRow(&s, th, v, v.scroll+i, y, width, ctx.Method)
		y++
	}
	if v.below > 0 {
		s.Print(0, y, indicator(glyphDown, v.below, width, ctx.Method), th.Muted, ctx.Method)
	}
	if p.notice != "" {
		s.Print(0, h-1, padTo(p.notice, width, ctx.Method), th.Warning, ctx.Method)
	}
	return s
}

// schedule asks for the next frame the band needs: a running or waiting row
// takes one a second so its action follows the child's calls, an ended row
// disappears when its window closes, and the footer hint expires on its own
// clock.
func (p *Panel) schedule(ctx components.DrawContext, v view) {
	now := p.clock()
	for _, r := range v.children {
		if r.State.terminal() {
			// The current screen's row outlives every window, so it needs no
			// frame to expire on — and asking for one already past would spin
			// the draw loop.
			if r.ID != p.current {
				ctx.WakeAt(r.Ended.Add(window))
			}
			continue
		}
		ctx.WakeIn(tick)
	}
	if now.Before(p.hintUntil) {
		ctx.WakeAt(p.hintUntil)
	}
}

// palette falls back to the default theme when the panel was built with a
// zero one, the way every other pane does, so a standalone draw is legible.
func (p *Panel) palette() components.Theme {
	th := p.theme
	if th.Foreground.Fg.Kind == 0 && th.Muted.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return th
}

// drawRow paints one list row: the head (markers, title, state word) in the
// foreground, the live action dim behind it, and the whole width reversed
// when the row is selected — the watch browser's selection, on a band.
func (p *Panel) drawRow(
	s *components.Surface,
	th components.Theme,
	v view,
	idx, y, width int,
	method xui.WidthMethod,
) {
	head, tail := p.rowText(v, idx, width, method)
	headStyle, tailStyle := th.Foreground, th.Muted
	if idx == p.cursor.Selected() {
		headStyle = xui.Style{Reverse: true}
		tailStyle = headStyle
	}
	x := s.Print(0, y, head, headStyle, method)
	x += s.Print(x, y, tail, tailStyle, method)
	if pad := width - x; pad > 0 {
		s.Print(x, y, strings.Repeat(" ", pad), tailStyle, method)
	}
}

// rowText composes one list row as a head and a dim tail. The title is what
// gives way when the row is too long: the state word and the action are the
// facts a narrow terminal must keep.
func (p *Panel) rowText(v view, idx, width int, method xui.WidthMethod) (head, tail string) {
	marker := glyphOther
	if v.id(idx) == p.current {
		marker = glyphCurrent
	}
	if idx <= 0 || idx > len(v.children) {
		return layout.TruncateToWidth(marker+" "+mainTitle, width, method), ""
	}

	r := v.children[idx-1]
	lead := marker + " " + r.State.glyph() + " "
	status := ""
	switch {
	case r.State == StateWaiting && r.Waiting != "":
		status = " · waiting: " + r.Waiting
	case r.State.label() != "":
		status = " · " + r.State.label()
	}
	tail = suffix(r)

	room := width - xui.StringWidth(lead+status+tail, method)
	head = lead + layout.EllipsizeToWidth(r.Title, room, method) + status
	head = layout.TruncateToWidth(head, width, method)
	tail = layout.TruncateToWidth(tail, max(width-xui.StringWidth(head, method), 0), method)
	return head, tail
}

// suffix is the row's live tail: what the running child is doing right now, in
// the transcript's own words for that call. A child that has made no call yet
// gets none — there is nothing to report — and an ended or waiting row gets
// none either, because its state word is the news.
func suffix(r Row) string {
	if r.State != StateRunning || r.Action == "" {
		return ""
	}
	return actionGap + r.Action
}

// indicator renders one "↑ N more" chrome row, padded so it reads as a row
// rather than as a stray label.
func indicator(glyph string, n, width int, method xui.WidthMethod) string {
	return padTo(glyph+" "+strconv.Itoa(n)+" more", width, method)
}

// padTo truncates s to width and pads it out with spaces.
func padTo(s string, width int, method xui.WidthMethod) string {
	s = layout.TruncateToWidth(s, width, method)
	if pad := width - xui.StringWidth(s, method); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}
