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
	"github.com/alvnukov/cozyphi/internal/tui/browse"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
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
	viewport int // item rows available, measured by the last Draw

	// The standard machinery: the motion parser, the cursor it drives, and
	// the armed y/n confirmation for trim and delete.
	motions browse.Motions
	cursor  browse.Cursor
	confirm browse.Confirm

	// The `/` fuzzy jump and the `.` action menu, both kit machines; the
	// menu replaces the item rows while open and keeps its own cursor so
	// the list selection survives the round trip.
	jump    browse.Jump
	menu    []browse.MenuItem
	menuCur browse.Cursor

	// Shift-range selection: anchor arms on the first extended move and the
	// range spans anchor..selected until a plain move collapses it.
	anchor  int
	ranging bool
	// notice is a one-keypress footer message: what a dead key should have
	// done. The next key clears it.
	notice string

	// Block viewer popup: the selected entry's full body, wrapped, scrolled.
	popup     bool
	popupText browse.Scroller

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
	p.motions.Reset()
	p.cursor.Apply(browse.Motion{Op: browse.OpBottom})
}

// Hide closes the browser, drops any pending confirmations, popup state and
// pending vim input, and notifies the shell so it can restore composer focus.
func (p *Pane) Hide() {
	if !p.visible {
		return
	}
	p.visible = false
	p.resetOverlays()
	p.motions.Reset()
	if p.onClose != nil {
		p.onClose()
	}
}

// resetOverlays clears every transitory overlay: confirmations, the shift
// range, the block viewer popup, the jump strip and the action menu.
func (p *Pane) resetOverlays() {
	p.notice = ""
	p.confirm.Disarm()
	p.ranging = false
	p.popup = false
	p.popupText.Jump(0)
	p.jump.Close()
	p.menu = nil
}

// Visible reports whether the browser covers the screen.
func (p *Pane) Visible() bool { return p.visible }

func (p *Pane) refresh() {
	if p.snapshot != nil {
		p.view = p.snapshot()
	}
	p.cursor.SetRows(len(p.view.Items), nil)
}

// selectedEntry returns the entry the actions act on, if any.
func (p *Pane) selectedEntry() (session.ContextItem, bool) {
	if len(p.view.Items) == 0 {
		return session.ContextItem{}, false
	}
	return p.view.Items[p.cursor.Selected()], true
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
	if p.jump.Active() {
		p.handleJumpEvent(ctx, ev)
		ctx.ConsumeAndRedraw()
		return true
	}
	switch e := ev.(type) {
	case xui.MouseEvent:
		// The pane covers the screen, so all clicks stay here; only the
		// wheel does anything — it scrolls the popup when one is open.
		if p.popup {
			if m, ok := browse.Wheel(e); ok {
				p.popupText.Apply(m)
			}
			ctx.ConsumeAndRedraw()
			return true
		}
		if p.menu != nil {
			p.menuCur.Wheel(e)
			ctx.ConsumeAndRedraw()
			return true
		}
		p.cursor.Wheel(e)
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
	p.notice = ""
	if p.popup {
		p.handlePopupKey(e)
		return
	}
	if p.menu != nil {
		p.handleMenuKey(e)
		return
	}
	// An armed confirmation gets the key first: y fires, n and Esc cancel,
	// and anything else withdraws the question and falls through — acting
	// elsewhere must never leave a stale y waiting.
	if p.confirm.Key(e) {
		return
	}
	// Shift-extended arrows are pane work, never motions.
	if e.Mods.Has(xui.ModShift) && (e.Code == xui.KeyUp || e.Code == xui.KeyDown) {
		p.motions.Reset()
		delta := 1
		if e.Code == xui.KeyUp {
			delta = -1
		}
		p.extendSelection(delta)
		return
	}
	if m, ok := p.motions.Key(e); ok {
		p.ranging = false
		p.cursor.Apply(m)
		return
	}
	switch e.Code {
	case xui.KeyEscape:
		p.Hide()
	case xui.KeyEnter:
		p.openPopup()
	case xui.KeyDelete:
		p.requestDelete()
	case xui.KeyBackspace:
		// Backspace deleted entries here once, which the footer never promised
		// and which reads as "go back" in a list. Say which keys delete instead
		// of destroying rows behind the footer's back.
		p.notice = "backspace does nothing here — press Del or d to delete"
	case xui.KeyRune:
		p.handleRune(e)
	}
}

// handleRune covers the pane's own letters; the motion dialect (j/k,
// counts, gg/G, Ctrl+U/D) is already claimed by the shared parser.
func (p *Pane) handleRune(e xui.KeyEvent) {
	if e.Mods != 0 {
		return
	}
	switch e.HotkeyRune() {
	case 'r':
		p.refresh()
	case 'd':
		p.requestDelete()
	case 'c':
		p.compact()
	case 't':
		p.requestTrim()
	case '/':
		p.openJump()
	case '.':
		p.openMenu()
	}
}

func (p *Pane) compact() {
	p.Hide()
	if p.onCompact != nil {
		p.onCompact()
	}
}

// openJump starts the `/` fuzzy jump over the item rows: the query is
// matched against each row's kind and preview, and the selection follows
// the tightest match live.
func (p *Pane) openJump() {
	if len(p.view.Items) == 0 {
		return
	}
	p.motions.Reset()
	p.confirm.Disarm()
	p.ranging = false
	p.jump.Open(p.cursor.Selected(), p.theme.Foreground, p.theme.Muted)
}

func (p *Pane) handleJumpEvent(ctx *components.EventContext, ev xui.Event) {
	result := p.jump.Handle(ctx, ev, &p.cursor, len(p.view.Items), func(i int) (string, bool) {
		item := p.view.Items[i]
		return item.Kind + " " + item.Preview, true
	})
	if result == browse.JumpClick {
		if mouse, ok := ev.(xui.MouseEvent); ok {
			p.cursor.Wheel(mouse)
		}
	}
}

// openMenu builds the `.` action menu: the commands for the selected entry
// plus the browser-wide ones, each naming the chord that runs it directly.
func (p *Pane) openMenu() {
	item, ok := p.selectedEntry()
	if !ok {
		return
	}
	p.motions.Reset()
	p.confirm.Disarm()
	items := []browse.MenuItem{{Label: "View block (Enter)", Run: p.openPopup}}
	if trimmable(item) {
		items = append(items, browse.MenuItem{Label: "Trim context up to here (t)", Run: p.requestTrim})
	}
	if n := len(p.deletableIDs()); n > 0 {
		label := "Delete block (Del)"
		if n > 1 {
			label = fmt.Sprintf("Delete %d blocks (Del)", n)
		}
		items = append(items, browse.MenuItem{Label: label, Run: p.requestDelete})
	}
	items = append(items,
		browse.MenuItem{Label: "Compact now (c)", Run: p.compact},
		browse.MenuItem{Label: "Refresh (r)", Run: p.refresh},
	)
	p.menu = items
	p.menuCur = browse.Cursor{}
	p.syncMenuCursor()
}

// syncMenuCursor re-teaches the menu cursor its rows: a title the cursor
// skips, then one row per command.
func (p *Pane) syncMenuCursor() {
	p.menuCur.SetRows(len(p.menu)+1, func(i int) bool { return i > 0 })
}

func (p *Pane) handleMenuKey(e xui.KeyEvent) {
	p.syncMenuCursor()
	if m, ok := p.motions.Key(e); ok {
		p.menuCur.Apply(m)
		return
	}
	switch e.Code {
	case xui.KeyEscape:
		p.menu = nil
	case xui.KeyEnter:
		p.runMenuItem()
	case xui.KeyRune:
		if e.Mods == 0 && e.Rune == ' ' {
			p.runMenuItem()
		}
	}
}

// runMenuItem leaves the menu first: the command runs on the list it came
// from, exactly as its chord would.
func (p *Pane) runMenuItem() {
	idx := p.menuCur.Selected() - 1
	if idx < 0 || idx >= len(p.menu) {
		return
	}
	item := p.menu[idx]
	p.menu = nil
	item.Run()
}

// requestTrim arms the trim confirmation for the selected entry, capturing
// its ID now: the question must fire on the row it named, not on wherever
// the cursor sits when y lands.
func (p *Pane) requestTrim() {
	item, ok := p.selectedEntry()
	if !ok || !trimmable(item) {
		return
	}
	id := item.EntryID
	p.confirm.Arm("trim context up to this entry?", func() {
		if p.onTrim != nil {
			_ = p.onTrim(id) // the shell toasts errors and refreshes
		}
		p.refresh()
	})
}

// requestDelete arms the delete confirmation for the deletable entries in
// the current selection (range or single). Summary rows never delete: they
// are the compaction that shapes the context itself.
func (p *Pane) requestDelete() {
	ids := p.deletableIDs()
	if len(ids) == 0 {
		return
	}
	p.confirm.Arm(fmt.Sprintf("delete %d block(s) from context?", len(ids)), func() {
		if p.onDelete != nil {
			_ = p.onDelete(ids) // the shell toasts errors and refreshes
		}
		// After a delete the remaining selection is a single row.
		p.ranging = false
		p.refresh()
	})
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
		return min(p.anchor, p.cursor.Selected()), max(p.anchor, p.cursor.Selected())
	}
	return p.cursor.Selected(), p.cursor.Selected()
}

// extendSelection moves the selection while growing the shift-range: the
// first extended move arms the anchor at the current row and the range
// spans anchor..selected until a plain move collapses it.
func (p *Pane) extendSelection(delta int) {
	if !p.ranging {
		p.anchor = p.cursor.Selected()
		p.ranging = true
	}
	p.cursor.MoveBy(delta)
}

// openPopup shows the selected block's full body over the list.
func (p *Pane) openPopup() {
	if _, ok := p.selectedEntry(); !ok {
		return
	}
	p.popup = true
	p.popupText.Jump(0)
	p.motions.Reset()
	p.confirm.Disarm()
}

// handlePopupKey drives the block viewer: the standard motions scroll,
// Enter/Escape/q return to the list. Every key is consumed so the list
// never reacts behind the popup.
func (p *Pane) handlePopupKey(e xui.KeyEvent) {
	if e.Code == xui.KeyEscape || e.Code == xui.KeyEnter ||
		(e.Code == xui.KeyRune && e.Mods == 0 && e.HotkeyRune() == 'q') {
		p.popup = false
		p.motions.Reset()
		return
	}
	if m, ok := p.motions.Key(e); ok {
		p.popupText.Apply(m)
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
	strip := 0
	if p.jump.Active() {
		strip = 2
	}
	p.viewport = max(h-chromeRows(p.view)-strip, 1)
	p.cursor.SetViewport(p.viewport)

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

	if p.menu != nil {
		// The action menu replaces the item rows: a title the cursor skips,
		// then one row per command, on the menu's own cursor.
		p.syncMenuCursor()
		p.menuCur.SetViewport(p.viewport)
		rows := make([]string, 0, len(p.menu)+1)
		rows = append(rows, "Actions")
		for _, item := range p.menu {
			rows = append(rows, "  "+item.Label)
		}
		for i := 0; i < p.viewport; i++ {
			idx := p.menuCur.Scroll() + i
			if idx >= len(rows) {
				break
			}
			style := th.Foreground
			marker := "  "
			if idx == p.menuCur.Selected() {
				style = xui.Style{Reverse: true}
				marker = "▶ "
			}
			s.Print(0, y, marker, style, ctx.Method)
			s.Print(2, y, layout.TruncateToWidth(rows[idx], w-4, ctx.Method), style, ctx.Method)
			y++
		}
	} else {
		for i := 0; i < p.viewport; i++ {
			idx := p.cursor.Scroll() + i
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
			if idx == p.cursor.Selected() {
				style = xui.Style{Reverse: true}
				marker = "▶ "
			}
			s.Print(0, y, marker, style, ctx.Method)
			s.Print(2, y, p.itemRow(idx, item, w-4, ctx.Method), style, ctx.Method)
			y++
		}
	}

	// Footer: hints, or the pending trim/delete confirmation.
	hint := keys.Footer(keys.ScopeContext)
	if p.menu != nil {
		hint = keys.Footer(keys.ScopeMenu)
	}
	if p.jump.Active() {
		hint = keys.Footer(keys.ScopeJump)
	}
	if p.confirm.Armed() {
		hint = " " + p.confirm.Label() + " (y/n)"
	} else if p.notice != "" {
		hint = " " + p.notice
	}
	hintStyle := th.Muted
	if p.confirm.Armed() || p.notice != "" {
		hintStyle = th.Warning
	}
	s.Print(1, h-1, layout.TruncateToWidth(hint, w-2, ctx.Method), hintStyle, ctx.Method)

	if strip > 0 {
		field := p.jump.Field().Draw(components.DrawContext{
			Max:    components.Size{Width: max(w-6, 1), Height: 1},
			Method: ctx.Method,
		})
		p.jump.DrawStrip(&s, field, ctx.Method, h-3, w, h, browse.StripStyle{
			Rule:   xui.Style{Fg: th.Muted.Fg},
			Label:  th.Foreground,
			Warn:   th.Warning,
			Prompt: th.Muted,
			Caps:   [2]string{"─", "─"},
		})
		if field.Cursor != nil {
			s.Cursor = &components.Point{X: 4 + field.Cursor.X, Y: h - 2 + field.Cursor.Y}
		}
	}

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
	p.popupText.SetExtent(len(lines), max(ph-2, 1))

	title := " " + item.Kind + " · " + tokensLabel(item.Tokens) + " "
	panel := components.NewSurface(pw, ph, nil)
	layout.DrawRoundedBorder(&panel, layout.BorderRounded, xui.Style{Fg: th.Muted.Fg},
		&layout.BorderLabel{Text: title, Style: xui.Style{Bold: true, Fg: th.Foreground.Fg}},
		nil, nil,
		&layout.BorderLabel{Text: keys.Footer(keys.ScopeContextRaw), Style: xui.Style{Fg: th.Muted.Fg}},
		method,
	)
	fill := xui.Style{Fg: th.Foreground.Fg}
	for row := 1; row < ph-1; row++ {
		for col := 1; col < pw-1; col++ {
			panel.SetCell(col, row, xui.Cell{Char: " ", Width: 1, Style: fill})
		}
	}
	for i, line := range lines[p.popupText.Offset():] {
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
		if v.MicroElidedResults > 0 {
			rec += fmt.Sprintf(" · %d elided", v.MicroElidedResults)
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
