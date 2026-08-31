// Package helppane renders the full-screen keyboard help (/help, F1). It owns
// no bindings of its own: every row comes from the keys catalog, so the screen
// cannot fall behind what the handlers actually do.
package helppane

import (
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/tui/browse"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

// rowKind tags a rendered line so Draw can style it without re-deriving what
// it came from.
type rowKind int

const (
	rowBlank rowKind = iota
	rowTitle
	rowNote
	rowBinding
)

type row struct {
	kind rowKind
	key  string
	text string
}

// Pane is the help screen. Mutated and rendered on the UI goroutine.
type Pane struct {
	theme components.Theme

	rows []row
	// keyW is the width of the key column — the widest key label — and the
	// width method it was measured with, since arrows and box glyphs are
	// terminal-dependent.
	keyW        int
	keyMethod   xui.WidthMethod
	keyMeasured bool

	visible  bool
	motions  browse.Motions
	view     browse.Scroller
	viewport int // body rows available, measured by the last Draw

	// onClose fires once whenever the pane stops being visible, so the shell
	// can hand the keyboard back to the composer.
	onClose func()
}

// New builds a hidden pane over the catalog as it stands.
func New(theme components.Theme, onClose func()) *Pane {
	return &Pane{theme: theme, onClose: onClose, rows: buildRows(keys.Groups())}
}

// buildRows flattens the catalog into display lines. A binding with no
// description is footer-only and stays out of the screen.
func buildRows(groups []keys.Group) []row {
	rows := make([]row, 0, 64)
	for _, g := range groups {
		if len(rows) > 0 {
			rows = append(rows, row{kind: rowBlank})
		}
		rows = append(rows, row{kind: rowTitle, text: g.Title})
		if g.Note != "" {
			rows = append(rows, row{kind: rowNote, text: g.Note})
		}
		for _, b := range g.Bindings {
			if b.Desc == "" {
				continue
			}
			rows = append(rows, row{kind: rowBinding, key: b.Label(), text: b.Desc})
		}
	}
	return rows
}

// keyWidth measures the key column under m, caching the result: the labels
// never change, but the terminal's width method can.
func (p *Pane) keyWidth(m xui.WidthMethod) int {
	if p.keyMeasured && p.keyMethod == m {
		return p.keyW
	}
	w := 0
	for _, r := range p.rows {
		if r.kind == rowBinding {
			w = max(w, xui.StringWidth(r.key, m))
		}
	}
	p.keyW, p.keyMethod, p.keyMeasured = w, m, true
	return w
}

// SetTheme restyles the screen.
func (p *Pane) SetTheme(th components.Theme) {
	if p != nil {
		p.theme = th
	}
}

// Show opens the help screen at the top.
func (p *Pane) Show() {
	if p == nil {
		return
	}
	p.visible = true
	p.motions.Reset()
	p.view.Jump(0)
}

// Hide closes the screen and notifies the shell so it can restore composer
// focus.
func (p *Pane) Hide() {
	if p == nil || !p.visible {
		return
	}
	p.visible = false
	if p.onClose != nil {
		p.onClose()
	}
}

// Visible reports whether the help screen covers the chat view.
func (p *Pane) Visible() bool { return p != nil && p.visible }

// Handle implements components.Widget; the editor owns dispatch and calls
// HandleEvent instead, so this entry point is intentionally inert.
func (*Pane) Handle(*components.EventContext, xui.Event) {}

// HandleEvent consumes every key and wheel event while the screen is up: it
// covers the chat view, so nothing underneath may act on a keypress.
func (p *Pane) HandleEvent(ctx *components.EventContext, ev xui.Event) bool {
	if p == nil || !p.visible {
		return false
	}
	switch e := ev.(type) {
	case xui.MouseEvent:
		if m, ok := browse.Wheel(e); ok {
			p.view.Apply(m)
		}
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
	if e.Code == xui.KeyEscape || e.Code == xui.KeyF1 ||
		(e.Code == xui.KeyRune && e.Mods == 0 && e.HotkeyRune() == 'q') {
		p.Hide()
		return
	}
	if m, ok := p.motions.Key(e); ok {
		p.view.Apply(m)
	}
}

// Draw renders the whole screen: title, grouped rows, footer.
func (p *Pane) Draw(ctx components.DrawContext) components.Surface {
	w, h := ctx.Max.Width, ctx.Max.Height
	if w <= 0 {
		w = 40
	}
	if h <= 0 {
		h = 24
	}
	p.viewport = max(h-2, 1)
	p.view.SetExtent(len(p.rows), p.viewport)

	th := p.theme
	if th.Foreground.Fg.Kind == 0 && th.Muted.Fg.Kind == 0 {
		th = components.DefaultTheme()
	}
	s := components.NewSurface(w, h, p)
	// Opaque background so the transcript does not bleed through.
	fill := xui.Style{Fg: th.Foreground.Fg}
	for row := range h {
		for col := range w {
			s.SetCell(col, row, xui.Cell{Char: " ", Width: 1, Style: fill})
		}
	}

	title := "Keyboard shortcuts"
	if more := len(p.rows) - p.viewport; more > 0 {
		title += "  ↑↓ for more"
	}
	s.Print(1, 0, layout.TruncateToWidth(title, w-2, ctx.Method), th.Warning, ctx.Method)

	for i := range p.viewport {
		idx := p.view.Offset() + i
		if idx >= len(p.rows) {
			break
		}
		p.drawRow(&s, th, p.rows[idx], 1+i, w, ctx.Method)
	}

	s.Print(1, h-1, layout.TruncateToWidth(keys.Footer(keys.ScopeHelp), w-2, ctx.Method),
		th.Muted, ctx.Method)
	return s
}

// drawRow paints one catalog line: a title, its note, or a key/description
// pair in two columns.
func (p *Pane) drawRow(s *components.Surface, th components.Theme, r row, y, w int, m xui.WidthMethod) {
	switch r.kind {
	case rowBlank:
		return
	case rowTitle:
		s.Print(1, y, layout.TruncateToWidth(r.text, w-2, m), th.Warning, m)
	case rowNote:
		s.Print(3, y, layout.TruncateToWidth(r.text, w-4, m), th.Muted, m)
	case rowBinding:
		// The key column is padded to a fixed width so the descriptions line
		// up down the whole screen, not just inside one group.
		keyW := p.keyWidth(m)
		keyCol := r.key + strings.Repeat(" ", max(keyW-xui.StringWidth(r.key, m), 0))
		s.Print(3, y, layout.TruncateToWidth(keyCol, w-4, m), th.Keybind, m)
		x := 3 + keyW + 2
		if x < w-1 {
			s.Print(x, y, layout.TruncateToWidth(r.text, w-1-x, m), th.Foreground, m)
		}
	}
}
