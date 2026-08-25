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
}

// New builds a hidden pane. All three seams are required: the pane never
// reaches into the engine or the session itself.
func New(
	theme components.Theme,
	snapshot func() agent.ContextView,
	onCompact func(),
	onTrim func(entryID string) error,
) *Pane {
	return &Pane{theme: theme, snapshot: snapshot, onCompact: onCompact, onTrim: onTrim}
}

// Show refreshes the snapshot and opens the browser at the newest entry.
func (p *Pane) Show() {
	p.refresh()
	p.visible = true
	p.confirm = false
	p.selected = max(len(p.view.Items)-1, 0)
	p.scroll = 0
	p.clampScroll()
}

// Hide closes the browser and drops any pending trim confirmation.
func (p *Pane) Hide() {
	p.visible = false
	p.confirm = false
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
}

func (p *Pane) clampScroll() {
	if p.viewport <= 0 {
		p.scroll = 0
		return
	}
	p.scroll = clamp(p.scroll, 0, max(len(p.view.Items)-p.viewport, 0))
	p.scroll = min(p.scroll, p.selected)
	if p.selected >= p.scroll+p.viewport {
		p.scroll = p.selected - p.viewport + 1
	}
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
// HandleKey instead, so this entry point is intentionally inert.
func (*Pane) Handle(*components.EventContext, xui.Event) {}

// HandleKey drives the browser while visible. It consumes every key press so
// typing never leaks into the composer underneath, plus wheel scroll.
func (p *Pane) HandleKey(ctx *components.EventContext, ev xui.Event) bool {
	if p == nil || !p.visible {
		return false
	}
	switch e := ev.(type) {
	case xui.MouseEvent:
		notches := max(e.Wheel, 1)
		switch e.Button {
		case xui.MouseWheelUp:
			p.scroll = max(p.scroll-notches, 0)
			p.clampScroll()
		case xui.MouseWheelDown:
			p.scroll += notches
			p.clampScroll()
		default:
			return false
		}
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyEvent:
		if !e.Press {
			return true
		}
		switch e.Code {
		case xui.KeyEscape, xui.KeyEnter:
			p.Hide()
			ctx.ConsumeAndRedraw()
			return true
		case xui.KeyUp:
			p.moveSelection(-1)
		case xui.KeyDown:
			p.moveSelection(1)
		case xui.KeyHome:
			p.selected = 0
			p.clampScroll()
		case xui.KeyEnd:
			p.selected = max(len(p.view.Items)-1, 0)
			p.clampScroll()
		case xui.KeyPageUp:
			p.moveSelection(-max(p.viewport-1, 1))
		case xui.KeyPageDown:
			p.moveSelection(max(p.viewport-1, 1))
		case xui.KeyRune:
			if e.Mods != 0 {
				return true
			}
			switch e.Rune {
			case 'j':
				p.moveSelection(1)
			case 'k':
				p.moveSelection(-1)
			case 'g':
				p.selected = 0
				p.clampScroll()
			case 'G':
				p.selected = max(len(p.view.Items)-1, 0)
				p.clampScroll()
			case 'r':
				p.refresh()
			case 'c':
				p.Hide()
				if p.onCompact != nil {
					p.onCompact()
				}
			case 't':
				if item, ok := p.selectedEntry(); ok && trimmable(item) {
					p.confirm = true
				}
			case 'y':
				p.confirmTrim()
				ctx.ConsumeAndRedraw()
				return true
			case 'n':
				p.confirm = false
			default:
				return true
			}
		default:
			return true
		}
		ctx.ConsumeAndRedraw()
		return true
	default:
		return false
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
	p.selected = clamp(p.selected+delta, 0, max(len(p.view.Items)-1, 0))
	p.clampScroll()
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
	p.clampScroll()

	th := p.theme
	if th.Foreground.Fg.Kind == 0 && th.Muted.Fg.Kind == 0 {
		th = components.DefaultTheme()
	}
	s := components.NewSurface(w, h, p)

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
	hint := " ↑↓ move  t trim to here  c compact  r refresh  esc close"
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

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
