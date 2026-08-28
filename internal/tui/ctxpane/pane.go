// Package ctxpane renders the full-screen context browser (/context): what
// the model receives on the next request, item by item, with token numbers,
// a block viewer popup, and three actions — compact now, trim-from-here and
// delete. The pane is a dumb view over an agent.ContextView snapshot; every
// mutation goes back through the seams injected at construction (refresh,
// onCompact, onTrim, onDelete).
package ctxpane

import (
	"fmt"
	"strconv"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/components/text"
	"github.com/alvnukov/cozyphi/internal/session"
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
	// onDelete drops exactly the given entries from the model's context.
	onDelete func(ids []string) error

	view     agent.ContextView
	visible  bool
	selected int
	scroll   int
	viewport int // item rows available, measured by the last Draw
	confirm  bool

	// Shift-range selection: anchor arms on the first extended move and the
	// range spans anchor..selected until a plain move collapses it.
	anchor  int
	ranging bool
	// pendingDrop carries the confirmed delete's targets to 'y'.
	pendingDrop   []string
	confirmDelete bool

	// Block viewer popup: the selected entry's full body, wrapped, scrolled.
	popup       bool
	popupScroll int
	popupRows   int // wrapped body lines, measured by the last Draw
	popupView   int // popup rows visible, measured by the last Draw

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
	onDelete func(ids []string) error,
	onClose func(),
) *Pane {
	return &Pane{
		theme: theme, snapshot: snapshot, onCompact: onCompact,
		onTrim: onTrim, onDelete: onDelete, onClose: onClose,
	}
}

// Show refreshes the snapshot and opens the browser at the newest entry.
func (p *Pane) Show() {
	p.refresh()
	p.visible = true
	p.resetOverlays()
	p.resetInput()
	p.selected = max(len(p.view.Items)-1, 0)
	p.scroll = 0
	p.followSelection()
}

// Hide closes the browser, drops any pending confirmations, popup state and
// pending vim input, and notifies the shell so it can restore composer focus.
func (p *Pane) Hide() {
	if !p.visible {
		return
	}
	p.visible = false
	p.resetOverlays()
	p.resetInput()
	if p.onClose != nil {
		p.onClose()
	}
}

// resetOverlays clears every transitory overlay: confirmations, the shift
// range and the block viewer popup.
func (p *Pane) resetOverlays() {
	p.confirm = false
	p.confirmDelete = false
	p.pendingDrop = nil
	p.ranging = false
	p.popup = false
	p.popupScroll = 0
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
		// wheel does anything — it scrolls the popup when one is open.
		if p.popup {
			switch e.Button {
			case xui.MouseWheelUp:
				p.popupScroll -= max(e.Wheel, 1)
			case xui.MouseWheelDown:
				p.popupScroll += max(e.Wheel, 1)
			}
			ctx.ConsumeAndRedraw()
			return true
		}
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
	if p.popup {
		p.handlePopupKey(e)
		return
	}
	switch e.Code {
	case xui.KeyEscape:
		p.Hide()
	case xui.KeyEnter:
		p.openPopup()
	case xui.KeyUp:
		p.resetInput()
		p.stepSelection(-1, e.Mods.Has(xui.ModShift))
	case xui.KeyDown:
		p.resetInput()
		p.stepSelection(1, e.Mods.Has(xui.ModShift))
	case xui.KeyDelete, xui.KeyBackspace:
		p.resetInput()
		p.requestDelete()
	case xui.KeyHome:
		p.resetInput()
		p.ranging = false
		p.selected = 0
		p.followSelection()
	case xui.KeyEnd:
		p.resetInput()
		p.ranging = false
		p.selected = max(len(p.view.Items)-1, 0)
		p.followSelection()
	case xui.KeyPageUp:
		p.resetInput()
		p.ranging = false
		p.moveSelection(-max(p.viewport-1, 1))
	case xui.KeyPageDown:
		p.resetInput()
		p.ranging = false
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
	r := e.HotkeyRune()
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
		p.ranging = false
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
			p.ranging = false
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
		p.ranging = false
		p.followSelection()
	case 'r':
		p.resetInput()
		p.refresh()
	case 'd':
		p.resetInput()
		p.requestDelete()
	case 'c':
		p.Hide()
		if p.onCompact != nil {
			p.onCompact()
		}
	case 't':
		p.resetInput()
		if item, ok := p.selectedEntry(); ok && trimmable(item) {
			// Only one confirmation can be armed at a time: a double 'y'
			// must not fire a delete and then a trim.
			p.confirmDelete = false
			p.pendingDrop = nil
			p.confirm = true
		}
	case 'y':
		if p.confirmDelete {
			p.confirmDeleteAction()
		} else {
			p.confirmTrim()
		}
	case 'n':
		p.resetInput()
		p.confirm = false
		p.confirmDelete = false
		p.pendingDrop = nil
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

// confirmDeleteAction fires the pending delete through the seam and drops
// the range: after a delete the remaining selection is a single row.
func (p *Pane) confirmDeleteAction() {
	if !p.confirmDelete || p.onDelete == nil {
		p.confirmDelete = false
		p.pendingDrop = nil
		return
	}
	_ = p.onDelete(p.pendingDrop) // the shell toasts errors and refreshes
	p.confirmDelete = false
	p.pendingDrop = nil
	p.ranging = false
	p.refresh()
}

// requestDelete arms the delete confirmation for the deletable entries in
// the current selection (range or single). Summary rows never delete: they
// are the compaction that shapes the context itself.
func (p *Pane) requestDelete() {
	ids := p.deletableIDs()
	if len(ids) == 0 {
		return
	}
	// Only one confirmation can be armed at a time: a double 'y' must not
	// fire a delete and then a trim.
	p.confirm = false
	p.pendingDrop = ids
	p.confirmDelete = true
}

// deletableIDs lists entry IDs the delete would drop, in list order.
func (p *Pane) deletableIDs() []string {
	lo, hi := p.selRange()
	var ids []string
	for i := lo; i <= hi && i < len(p.view.Items); i++ {
		if i < 0 {
			continue
		}
		if item := p.view.Items[i]; item.Kind != "summary" {
			ids = append(ids, item.EntryID)
		}
	}
	return ids
}

// selRange returns the selection bounds: the shift-range when armed, else
// the single selected row.
func (p *Pane) selRange() (int, int) {
	if p.ranging {
		return min(p.anchor, p.selected), max(p.anchor, p.selected)
	}
	return p.selected, p.selected
}

// moveSelection moves the selection without extending a range.
func (p *Pane) moveSelection(delta int) {
	p.stepSelection(delta, false)
}

// stepSelection moves the selection by delta. With extend, the first move
// arms the anchor at the current row and the range spans anchor..selected
// until a plain move collapses it back to a single row.
func (p *Pane) stepSelection(delta int, extend bool) {
	if extend {
		if !p.ranging {
			p.anchor = p.selected
			p.ranging = true
		}
	} else {
		p.ranging = false
	}
	p.selected = clamp01(p.selected+delta, max(len(p.view.Items)-1, 0))
	p.followSelection()
}

// openPopup shows the selected block's full body over the list.
func (p *Pane) openPopup() {
	if _, ok := p.selectedEntry(); !ok {
		return
	}
	p.popup = true
	p.popupScroll = 0
	p.confirm = false
	p.confirmDelete = false
	p.pendingDrop = nil
}

// handlePopupKey drives the block viewer: arrows/j/k scroll, Enter/Escape/q
// return to the list. Every key is consumed so the list never reacts behind
// the popup.
func (p *Pane) handlePopupKey(e xui.KeyEvent) {
	step := max(p.popupView-1, 1)
	switch e.Code {
	case xui.KeyEscape, xui.KeyEnter:
		p.popup = false
	case xui.KeyUp:
		p.popupScroll--
	case xui.KeyDown:
		p.popupScroll++
	case xui.KeyPageUp:
		p.popupScroll -= step
	case xui.KeyPageDown:
		p.popupScroll += step
	case xui.KeyHome:
		p.popupScroll = 0
	case xui.KeyEnd:
		p.popupScroll = max(p.popupRows-p.popupView, 0)
	case xui.KeyRune:
		switch e.HotkeyRune() {
		case 'j':
			p.popupScroll++
		case 'k':
			p.popupScroll--
		case 'q':
			p.popup = false
		}
	}
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
		if lo, hi := p.selRange(); p.ranging && idx >= lo && idx <= hi {
			style = xui.Style{Reverse: true}
			marker = "◈ "
		}
		if idx == p.selected {
			style = xui.Style{Reverse: true}
			marker = "▶ "
		}
		s.Print(0, y, marker, style, ctx.Method)
		s.Print(2, y, p.itemRow(idx, item, w-4, ctx.Method), style, ctx.Method)
		y++
	}

	// Footer: hints, or the pending trim/delete confirmation.
	hint := " ↑↓ move · shift+↑↓ select · enter view · del delete · t trim · c compact · r refresh · esc close"
	if p.confirmDelete {
		hint = fmt.Sprintf(" delete %d block(s) from context?  y confirm · n cancel", len(p.pendingDrop))
	} else if p.confirm {
		hint = " trim context up to this entry?  y confirm · n cancel"
	}
	hintStyle := th.Muted
	if p.confirm || p.confirmDelete {
		hintStyle = th.Warning
	}
	s.Print(1, h-1, layout.TruncateToWidth(hint, w-2, ctx.Method), hintStyle, ctx.Method)

	if p.popup {
		p.drawPopup(&s, th, w, h, ctx.Method)
	}
	return s
}

// drawPopup paints the centered block viewer over the list: a bordered panel
// with the selected entry's wrapped body, scrolled to popupScroll. The popup
// is blitted into the pane's own buffer so the whole browser stays one
// surface — no extra z-order layer for the shell to manage.
func (p *Pane) drawPopup(s *components.Surface, th components.Theme, w, h int, method xui.WidthMethod) {
	item, ok := p.selectedEntry()
	if !ok {
		p.popup = false // nothing to show; drop the popup instead of crashing
		return
	}
	pw, ph := min(w-4, 72), min(h-4, 20)
	if pw < 10 || ph < 4 {
		p.popup = false // terminal too small for a readable panel
		return
	}
	lines := text.WrapEditorLines(item.Body, pw-4, method)
	p.popupRows = len(lines)
	p.popupView = max(ph-2, 1)
	p.popupScroll = clamp01(p.popupScroll, max(p.popupRows-p.popupView, 0))

	title := " " + item.Kind + " · " + tokensLabel(item.Tokens) + " "
	panel := components.NewSurface(pw, ph, nil)
	layout.DrawRoundedBorder(&panel, layout.BorderRounded, xui.Style{Fg: th.Muted.Fg},
		&layout.BorderLabel{Text: title, Style: xui.Style{Bold: true, Fg: th.Foreground.Fg}},
		nil, nil,
		&layout.BorderLabel{Text: " j/k scroll · enter close ", Style: xui.Style{Fg: th.Muted.Fg}},
		method,
	)
	fill := xui.Style{Fg: th.Foreground.Fg}
	for row := 1; row < ph-1; row++ {
		for col := 1; col < pw-1; col++ {
			panel.SetCell(col, row, xui.Cell{Char: " ", Width: 1, Style: fill})
		}
	}
	for i, line := range lines[p.popupScroll:] {
		if 1+i >= ph-1 {
			break
		}
		panel.Print(2, 1+i, layout.TruncateToWidth(line, pw-4, method), th.Foreground, method)
	}

	x0, y0 := (w-pw)/2, (h-ph)/2
	for row := range ph {
		for col := range pw {
			s.SetCell(x0+col, y0+row, panel.Buffer[row*pw+col])
		}
	}
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

// itemRow renders one context row; idx is the item's position in the view,
// shown 1-based — one entry can span several items, so the label numbers
// rows, not entries.
func (p *Pane) itemRow(idx int, item session.ContextItem, w int, method xui.WidthMethod) string {
	cumPct := 0
	if p.view.EstimatedTokens > 0 {
		cumPct = item.CumulativeTokens * 100 / p.view.EstimatedTokens
	}
	row := fmt.Sprintf("%3d  %-9s %5s  %3d%%  ", idx+1, item.Kind, tokensLabel(item.Tokens), cumPct)
	preview := item.Preview
	if preview == "" {
		preview = "(empty)"
	}
	return row + layout.TruncateToWidth(preview, max(w-len(row), 1), method)
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
