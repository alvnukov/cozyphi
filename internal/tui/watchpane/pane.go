// Package watchpane renders the full-screen watch browser (/watches): every
// watch this session started with its state, event count and age, a log
// viewer popup, and stop-with-confirm. The pane is a dumb view over a
// watch.Watch snapshot re-read on every Draw; every mutation goes back
// through the seams injected at construction (log, onStop, onClose).
package watchpane

import (
	"fmt"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/components/text"
	"github.com/alvnukov/cozyphi/internal/tui/browse"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
	"github.com/alvnukov/cozyphi/internal/watch"
)

// logLimit caps how many events the log popup asks for: a flooded watch
// floods no further than this.
const logLimit = 200

// chromeRows counts non-list rows: header, column titles, and the footer hint.
const chromeRows = 3

// Pane is the watch browser. Mutated and rendered on the UI goroutine.
type Pane struct {
	theme components.Theme

	// snapshot re-pulls the watch list; log fetches one watch's tail; onStop
	// ends a live watch; onClose fires once when the pane stops being
	// visible, so the shell can hand the keyboard back.
	snapshot func() []watch.Watch
	log      func(id string, limit int) ([]watch.Event, error)
	onStop   func(id string) error
	onClose  func()

	visible  bool
	viewport int // list rows available, measured by the last Draw

	// The standard machinery: the motion parser, the cursor it drives, and
	// the armed y/n confirmation for stop.
	motions browse.Motions
	cursor  browse.Cursor
	confirm browse.Confirm

	// notice is a one-keypress footer message: what a dead key should have
	// done. The next key clears it.
	notice string

	// Log viewer popup: the selected watch's tail, wrapped, scrolled. The
	// lines are fetched once at open — the pane shows what happened up to
	// the moment the user asked, like watch action=log.
	popup      bool
	popupLines []string
	popupText  browse.Scroller
}

// New builds a hidden pane. The pane never reaches into the watch manager:
// every read and side effect goes back through these seams.
func New(
	theme components.Theme,
	snapshot func() []watch.Watch,
	log func(id string, limit int) ([]watch.Event, error),
	onStop func(id string) error,
	onClose func(),
) *Pane {
	return &Pane{
		theme: theme, snapshot: snapshot, log: log,
		onStop: onStop, onClose: onClose,
	}
}

// Show opens the browser at the first row.
func (p *Pane) Show() {
	p.visible = true
	p.list() // prime the cursor's rows so selection works before the first Draw
	p.resetOverlays()
	p.motions.Reset()
	p.cursor.Apply(browse.Motion{Op: browse.OpTop})
}

// Hide closes the browser, drops any pending confirmation, popup state and
// pending vim input, and notifies the shell so it can restore focus.
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

func (p *Pane) resetOverlays() {
	p.notice = ""
	p.confirm.Disarm()
	p.popup = false
	p.popupLines = nil
	p.popupText.Jump(0)
}

// Visible reports whether the browser covers the screen.
func (p *Pane) Visible() bool { return p.visible }

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
	// An armed confirmation gets the key first: y fires, n and Esc cancel,
	// and anything else withdraws the question and falls through.
	if p.confirm.Key(e) {
		return
	}
	if m, ok := p.motions.Key(e); ok {
		p.cursor.Apply(m)
		return
	}
	switch e.Code {
	case xui.KeyEscape:
		p.Hide()
	case xui.KeyEnter:
		p.openPopup()
	case xui.KeyRune:
		if e.Mods != 0 {
			return
		}
		p.handleRune(e)
	}
}

// handleRune covers the pane's own letters; the motion dialect (j/k, counts,
// gg/G, Ctrl+U/D) is already claimed by the shared parser.
func (p *Pane) handleRune(e xui.KeyEvent) {
	switch e.HotkeyRune() {
	case 'q':
		p.Hide()
	case 'r':
		p.notice = "refreshed"
	case 's':
		p.requestStop()
	}
}

// requestStop arms the stop confirmation for the selected watch, capturing
// its ID and label now: the question must fire on the row it named, not on
// wherever the cursor sits when y lands.
func (p *Pane) requestStop() {
	w, ok := p.selected()
	if !ok {
		return
	}
	if !w.Live {
		p.notice = "this watch already ended — nothing to stop"
		return
	}
	id, label := w.ID, w.Label
	p.confirm.Arm(fmt.Sprintf("stop watch %q?", label), func() {
		if p.onStop != nil {
			_ = p.onStop(id) // the shell toasts errors
		}
	})
}

// selected returns the watch under the cursor, if any.
func (p *Pane) selected() (watch.Watch, bool) {
	ws := p.list()
	if len(ws) == 0 {
		return watch.Watch{}, false
	}
	return ws[p.cursor.Selected()], true
}

// list re-reads the snapshot and re-teaches the cursor its rows. Draw calls
// it every frame; the list keeps moving underneath the pane, and the cursor
// must never point past the end.
func (p *Pane) list() []watch.Watch {
	if p.snapshot == nil {
		return nil
	}
	ws := p.snapshot()
	p.cursor.SetRows(len(ws), nil)
	return ws
}

// openPopup fetches the selected watch's event tail and shows it over the
// list. A failed fetch is a notice, not a crash — the watch may have ended
// and been pruned between the list and the fetch.
func (p *Pane) openPopup() {
	w, ok := p.selected()
	if !ok {
		return
	}
	events := []watch.Event(nil)
	if p.log != nil {
		var err error
		events, err = p.log(w.ID, logLimit)
		if err != nil {
			p.notice = "log unavailable: " + err.Error()
			return
		}
	}
	lines := make([]string, 0, len(events))
	for _, ev := range events {
		lines = append(lines, ev.Time.Format("15:04:05")+" "+ev.Text)
	}
	p.popup = true
	p.popupLines = lines
	p.popupText.Jump(0)
	p.motions.Reset()
	p.confirm.Disarm()
}

// handlePopupKey drives the log viewer: the standard motions scroll,
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

// Draw renders the whole screen: header, column titles, watch rows, footer.
func (p *Pane) Draw(ctx components.DrawContext) components.Surface {
	w, h := ctx.Max.Width, ctx.Max.Height
	if w <= 0 {
		w = 40
	}
	if h <= 0 {
		h = 24
	}
	ws := p.list()
	p.viewport = max(h-chromeRows, 1)
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
	s.Print(1, y, layout.TruncateToWidth(p.header(ws), w-2, ctx.Method), th.Warning, ctx.Method)
	y++
	s.Print(1, y, "  state     label                   events  age", th.Muted, ctx.Method)
	y++

	if len(ws) == 0 {
		s.Print(1, y, " no watches this session — start one through the agent", th.Muted, ctx.Method)
		y++
	}
	for i := 0; i < p.viewport; i++ {
		idx := p.cursor.Scroll() + i
		if idx >= len(ws) {
			break
		}
		item := ws[idx]
		style := th.Foreground
		marker := "  "
		if idx == p.cursor.Selected() {
			style = xui.Style{Reverse: true}
			marker = "▶ "
		}
		s.Print(0, y, marker, style, ctx.Method)
		s.Print(2, y, p.row(item, w-4, ctx.Method), style, ctx.Method)
		y++
	}

	// Footer: the catalog's own hint row, or the pending stop confirmation.
	hint := " " + keys.Hints(keys.ScopeWatches)
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

	if p.popup {
		p.drawPopup(&s, th, w, h, ctx.Method)
	}
	return s
}

// header counts live watches: the row the footer indicator summarizes,
// spelled out.
func (p *Pane) header(ws []watch.Watch) string {
	live := 0
	for _, w := range ws {
		if w.Live {
			live++
		}
	}
	return fmt.Sprintf(" Watches  %d live · %d total", live, len(ws))
}

// row renders one watch row: state, label, event count, age since start.
func (p *Pane) row(w watch.Watch, width int, method xui.WidthMethod) string {
	state := "ended"
	if w.Live {
		state = "running"
	} else if w.Err != "" {
		state = "failed"
	}
	label := w.Label
	if label == "" {
		label = "(unlabeled)"
	}
	age := ""
	if !w.Started.IsZero() {
		age = components.FormatDuration(time.Since(w.Started))
	}
	head := fmt.Sprintf("%-8s  %-22s %6d  %s", state, label, w.Events, age)
	return layout.TruncateToWidth(head, width, method)
}

// drawPopup paints the centered log viewer over the list: a bordered panel
// with the watch's wrapped tail, scrolled. The popup is blitted into the
// pane's own buffer so the whole browser stays one surface.
func (p *Pane) drawPopup(s *components.Surface, th components.Theme, w, h int, method xui.WidthMethod) {
	pw, ph := min(w-4, 72), min(h-4, 20)
	if pw < 10 || ph < 4 {
		p.popup = false // terminal too small for a readable panel
		return
	}
	lines := text.WrapEditorLines(strings.Join(p.popupLines, "\n"), pw-4, method)
	p.popupText.SetExtent(len(lines), max(ph-2, 1))

	wid, _ := p.selected()
	title := " " + wid.Label + " · log "
	panel := components.NewSurface(pw, ph, nil)
	layout.DrawRoundedBorder(&panel, layout.BorderRounded, xui.Style{Fg: th.Muted.Fg},
		&layout.BorderLabel{Text: title, Style: xui.Style{Bold: true, Fg: th.Foreground.Fg}},
		nil, nil,
		&layout.BorderLabel{Text: " j/k scroll · Esc back ", Style: xui.Style{Fg: th.Muted.Fg}},
		method,
	)
	fill := xui.Style{Fg: th.Foreground.Fg}
	for row := 1; row < ph-1; row++ {
		for col := 1; col < pw-1; col++ {
			panel.SetCell(col, row, xui.Cell{Char: " ", Width: 1, Style: fill})
		}
	}
	if len(lines) == 0 {
		panel.Print(2, 1, "(no events yet)", th.Muted, method)
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
