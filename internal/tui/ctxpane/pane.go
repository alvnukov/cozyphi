// Package ctxpane renders the full-screen context browser (/context): what
// the model receives on the next request, item by item, with token numbers
// and two actions — compact now and trim-from-here. The pane is a dumb view
// over an agent.ContextView snapshot; every mutation goes back through the
// seams injected at construction (refresh, onCompact, onTrim).
package ctxpane

import (
	"fmt"
	"strconv"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/agent"
	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/layout"
	"github.com/pulseaiclub/phi/internal/session"
)

// Pane is the context browser. Mutated and rendered on the UI goroutine.
type Pane struct {
	theme components.Theme

	// snapshot re-pulls the browser view from the engine.
	snapshot func() agent.ContextView
	// onCompact asks the shell to run a user-initiated compaction now.
	onCompact func()
	// onTrim drops everything before the entry from the model's context.
	onTrim func(entryID string) error

	view     agent.ContextView
	visible  bool
	selected int
	scroll   int
	viewport int // item rows available, measured by the last Draw
	confirm  bool

	// vim-style pending input: a count prefix ("3j") and a half-typed "gg".
	count    int
	pendingG bool

	// onClose fires once whenever the pane stops being visible, so the
	// shell can hand the keyboard back to the composer.
	onClose func()
}

// New builds a hidden pane. The pane never reaches into the engine or the
// session itself: every side effect goes back through these seams.
func New(
	theme components.Theme,
	snapshot func() agent.ContextView,
	onCompact func(),
	onTrim func(entryID string) error,
	onClose func(),
) *Pane {
	return &Pane{theme: theme, snapshot: snapshot, onCompact: onCompact, onTrim: onTrim, onClose: onClose}
}

// Show refreshes the snapshot and opens the browser at the newest entry.
func (p *Pane) Show() {
	p.refresh()
	p.visible = true
	p.confirm = false
	p.resetInput()
	p.selected = max(len(p.view.Items)-1, 0)
	p.scroll = 0
	p.followSelection()
}

// Hide closes the browser, drops any pending trim confirmation and pending
// vim input, and notifies the shell so it can restore composer focus.
func (p *Pane) Hide() {
	if !p.visible {
		return
	}
	p.visible = false
	p.confirm = false
	p.resetInput()
	if p.onClose != nil {
		p.onClose()
	}
}

// resetInput clears pending vim-style input (count prefix, half-typed gg).
func (p *Pane) resetInput() {
	p.count = 0
	p.pendingG = false
}

// step consumes the pending count, defaulting to one.
func (p *Pane) step() int {
	n := max(p.count, 1)
	p.count = 0
	return n
}

// Visible reports whether the browser covers the screen.
func (p *Pane) Visible() bool { return p.visible }

func (p *Pane) refresh() {
	if p.snapshot != nil {
		p.view = p.snapshot()
	}
	if p.selected >= len(p.view.Items) {
		p.selected = max(len(p.view.Items)-1, 0)
	}
	p.clampScroll()
	p.followSelection()
}

func (p *Pane) clampScroll() {
	if p.viewport <= 0 {
		p.scroll = 0
		return
	}
	p.scroll = clamp01(p.scroll, max(len(p.view.Items)-p.viewport, 0))
}

// followSelection scrolls the minimum needed to bring the selection back
// into view. Wheel scrolling never calls this: the viewport moves freely
// instead of snapping back to the selected row.
func (p *Pane) followSelection() {
	if p.viewport <= 0 {
		p.scroll = 0
		return
	}
	p.scroll = min(p.scroll, p.selected)
	if p.selected >= p.scroll+p.viewport {
		p.scroll = p.selected - p.viewport + 1
	}
	p.clampScroll()
}

// selectedEntry returns the entry the actions act on, if any.
func (p *Pane) selectedEntry() (session.ContextItem, bool) {
	if p.selected < 0 || p.selected >= len(p.view.Items) {
		return session.ContextItem{}, false
	}
	return p.view.Items[p.selected], true
}

// trimmable reports whether trimming up to this entry makes sense (summary
// rows already describe dropped history; trimming onto one is a no-op).
func trimmable(item session.ContextItem) bool { return item.Kind != "summary" }

// Handle implements components.Widget; the editor owns dispatch and calls
// HandleEvent instead, so this entry point is intentionally inert.
func (*Pane) Handle(*components.EventContext, xui.Event) {}

// HandleEvent drives the browser while visible. It consumes every key press
// and mouse event so nothing leaks into the shell underneath.
func (p *Pane) HandleEvent(ctx *components.EventContext, ev xui.Event) bool {
	if p == nil || !p.visible {
		return false
	}
	switch e := ev.(type) {
	case xui.MouseEvent:
		// The pane covers the screen, so all clicks stay here; only the
		// wheel does anything.
		notches := max(e.Wheel, 1)
		switch e.Button {
		case xui.MouseWheelUp:
			p.scroll -= notches
		case xui.MouseWheelDown:
			p.scroll += notches
		}
		p.clampScroll()
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyEvent:
		if !e.Press {
			return true
		}
		p.handleKey(e)
		ctx.ConsumeAndRedraw()
		return true
	default:
		return false
	}
}

func (p *Pane) handleKey(e xui.KeyEvent) {
	switch e.Code {
	case xui.KeyEscape, xui.KeyEnter:
		p.Hide()
	case xui.KeyUp:
		p.resetInput()
		p.moveSelection(-1)
	case xui.KeyDown:
		p.resetInput()
		p.moveSelection(1)
	case xui.KeyHome:
		p.resetInput()
		p.selected = 0
		p.followSelection()
	case xui.KeyEnd:
		p.resetInput()
		p.selected = max(len(p.view.Items)-1, 0)
		p.followSelection()
	case xui.KeyPageUp:
		p.resetInput()
		p.moveSelection(-max(p.viewport-1, 1))
	case xui.KeyPageDown:
		p.resetInput()
		p.moveSelection(max(p.viewport-1, 1))
	case xui.KeyRune:
		p.handleRune(e)
	}
}

// handleRune decodes vim-style input. Counts accumulate ("3j"), "g" waits
// for its second "g", and Ctrl+d/Ctrl+u move half a page. Shift+letter
// arrives as a capital rune with ModShift, so the modifier guard must let
// it through instead of swallowing it.
func (p *Pane) handleRune(e xui.KeyEvent) {
	r := e.Rune
	switch {
	case e.Mods == xui.ModCtrl && (r == 'd' || r == 'u'):
		p.resetInput()
		half := max(p.viewport/2, 1)
		if r == 'd' {
			p.moveSelection(half)
		} else {
			p.moveSelection(-half)
		}
	case e.Mods == xui.ModShift && r == 'G':
		last := max(len(p.view.Items)-1, 0)
		if n := p.count - 1; p.count > 0 {
			p.selected = clamp01(n, last)
		} else {
			p.selected = last
		}
		p.resetInput()
		p.followSelection()
	case e.Mods != 0:
		p.resetInput()
	default:
		p.handlePlainRune(r)
	}
}

func (p *Pane) handlePlainRune(r rune) {
	if r >= '0' && r <= '9' {
		p.count = p.count*10 + int(r-'0')
		p.pendingG = false
		return
	}
	switch r {
	case 'j':
		p.moveSelection(p.step())
	case 'k':
		p.moveSelection(-p.step())
	case 'g':
		if p.pendingG {
			p.resetInput()
			p.selected = 0
			p.scroll = 0
		} else {
			p.pendingG = true
		}
	case 'G':
		last := max(len(p.view.Items)-1, 0)
		if n := p.count - 1; p.count > 0 {
			p.selected = clamp01(n, last)
		} else {
			p.selected = last
		}
		p.resetInput()
		p.followSelection()
	case 'r':
		p.resetInput()
		p.refresh()
	case 'c':
		p.Hide()
		if p.onCompact != nil {
			p.onCompact()
		}
	case 't':
		p.resetInput()
		if item, ok := p.selectedEntry(); ok && trimmable(item) {
			p.confirm = true
		}
	case 'y':
		p.confirmTrim()
	case 'n':
		p.resetInput()
		p.confirm = false
	default:
		p.resetInput()
	}
}

func (p *Pane) confirmTrim() {
	item, ok := p.selectedEntry()
	if !ok || !p.confirm || p.onTrim == nil {
		p.confirm = false
		return
	}
	_ = p.onTrim(item.EntryID) // the shell toasts errors and refreshes
	p.confirm = false
	p.refresh()
}

func (p *Pane) moveSelection(delta int) {
	p.selected = clamp01(p.selected+delta, max(len(p.view.Items)-1, 0))
	p.followSelection()
}

// Draw renders the whole screen: header, column titles, item rows, footer.
func (p *Pane) Draw(ctx components.DrawContext) components.Surface {
	w, h := ctx.Max.Width, ctx.Max.Height
	if w <= 0 {
		w = 40
	}
	if h <= 0 {
		h = 24
	}
	p.viewport = max(h-chromeRows(p.view), 1)
	// Bounds only: re-following here would drag free wheel scrolling back
	// to the selected row on every frame.
	p.clampScroll()

	th := p.theme
	if th.Foreground.Fg.Kind == 0 && th.Muted.Fg.Kind == 0 {
		th = components.DefaultTheme()
	}
	s := components.NewSurface(w, h, p)
	// Opaque background so the transcript does not bleed through.
	fill := xui.Style{Fg: th.Foreground.Fg}
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			s.SetCell(col, row, xui.Cell{Char: " ", Width: 1, Style: fill})
		}
	}

	y := 0
	s.Print(1, y, layout.TruncateToWidth(p.header(), w-2, ctx.Method), th.Warning, ctx.Method)
	y++
	if extra := p.compactionLine(); extra != "" {
		s.Print(1, y, layout.TruncateToWidth(extra, w-2, ctx.Method), th.Muted, ctx.Method)
		y++
	}
	s.Print(1, y, "  #  kind       ~tok    cum  preview", th.Muted, ctx.Method)
	y++

	for i := 0; i < p.viewport; i++ {
		idx := p.scroll + i
		if idx >= len(p.view.Items) {
			break
		}
		item := p.view.Items[idx]
		style := th.Foreground
		marker := "  "
		if idx == p.selected {
			style = xui.Style{Reverse: true}
			marker = "▶ "
		}
		s.Print(0, y, marker, style, ctx.Method)
		s.Print(2, y, p.itemRow(item, w-4, ctx.Method), style, ctx.Method)
		y++
	}

	// Footer: hints, or the pending trim confirmation.
	hint := " j/k·↑↓ move  gg/G ends  ^d/^u half page  t trim  c compact  r refresh  esc close"
	if p.confirm {
		hint = " trim context up to this entry?  y confirm · n cancel"
	}
	hintStyle := th.Muted
	if p.confirm {
		hintStyle = th.Warning
	}
	s.Print(1, h-1, layout.TruncateToWidth(hint, w-2, ctx.Method), hintStyle, ctx.Method)
	return s
}

func (p *Pane) header() string {
	v := p.view
	switch {
	case v.ContextWindow > 0:
		pct := 0
		if v.ContextWindow > 0 {
			pct = v.ContextTokens * 100 / v.ContextWindow
		}
		rec := ""
		if v.CompactionRecommended {
			rec = " · compaction recommended"
		}
		return fmt.Sprintf(
			" Context  %s tokens (%s) of %dk · %d%%%s",
			tokensLabel(v.ContextTokens), v.TokenSource, v.ContextWindow/1000, pct, rec,
		)
	case v.EstimatedTokens > 0:
		return fmt.Sprintf(" Context  ~%s tokens (estimate) · window unknown", tokensLabel(v.EstimatedTokens))
	default:
		return " Context  empty"
	}
}

func (p *Pane) compactionLine() string {
	if p.view.LastCompaction == nil {
		return ""
	}
	c := p.view.LastCompaction
	if c.FromTrim {
		return " last shaped by: manual trim"
	}
	if c.MessagesSummarized > 0 || c.TokensBefore > 0 {
		return " last shaped by: " + c.Report()
	}
	return " last shaped by: compaction — " + truncateRunes(c.Summary, 60)
}

func (p *Pane) itemRow(item session.ContextItem, w int, method xui.WidthMethod) string {
	cumPct := 0
	if p.view.EstimatedTokens > 0 {
		cumPct = item.CumulativeTokens * 100 / p.view.EstimatedTokens
	}
	row := fmt.Sprintf("%3d  %-9s %5s  %3d%%  ", indexLabel(p.view, item), item.Kind, tokensLabel(item.Tokens), cumPct)
	preview := item.Preview
	if preview == "" {
		preview = "(empty)"
	}
	return row + layout.TruncateToWidth(preview, max(w-len(row), 1), method)
}

// indexLabel shows the 1-based context position of the entry.
func indexLabel(v agent.ContextView, item session.ContextItem) int {
	for i, it := range v.Items {
		if it.EntryID == item.EntryID {
			return i + 1
		}
	}
	return 0
}

// chromeRows counts non-item rows: header, optional compaction line, column
// titles, and the footer hint.
func chromeRows(v agent.ContextView) int {
	n := 3 // header + titles + footer
	if v.LastCompaction != nil {
		n++
	}
	return n
}

// truncateRunes cuts s to at most n runes.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func tokensLabel(n int) string {
	switch {
	case n >= 1000000:
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	case n >= 1000:
		return fmt.Sprintf("%dk", n/1000)
	default:
		return strconv.Itoa(n)
	}
}

// clamp01 clamps v into the range [0, hi].
func clamp01(v, hi int) int {
	if v < 0 {
		return 0
	}
	if v > hi {
		return hi
	}
	return v
}
