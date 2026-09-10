// Package agentlist renders the full-screen sub-agent browser (/agents): every
// child this session ever started, the working ones on top and the finished
// ones below, each with its status, tool count, elapsed time and the one line
// it came back with. Not to be confused with internal/tui/agentpanel, the
// three-row band under the composer that shows only the live children.
//
// The pane is a dumb view over a snapshot re-read on every Draw: it never
// reaches into the job manager or a session, and every side effect — open one
// child's session, stop a running one, hand the keyboard back — leaves through
// the seams injected at construction. Motions, the cursor and the y/n
// confirmation come from internal/tui/browse and the hint row from
// internal/tui/keys, as internal/tui/DESIGN.md requires.
package agentlist

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/tui/browse"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

// chromeRows counts the non-list rows: the header and the footer hint.
const chromeRows = 2

// tick is how often a running row redraws so its elapsed time moves. The frame
// is asked for from inside Draw; the pane owns no timer.
const tick = time.Second

// titleFloor is the narrowest a title is squeezed to before the facts after it
// start giving way instead.
const titleFloor = 12

// The row glyphs. They are the band's, so one child reads the same in both
// places.
const (
	glyphRunning = "⟳"
	glyphWaiting = "⏸"
	glyphDone    = "✓"
	glyphFailed  = "✗"
	glyphStopped = "■"
)

// Agent is one child as the wiring sees it right now. Every field is a fact
// somebody else recorded: an empty one is a fact the job never had, and the row
// leaves it out rather than inventing a stand-in.
type Agent struct {
	JobID string
	// Title is the name every surface gives this child, role(description).
	Title string
	// Status is the job's own lifecycle status; "" is a child the manager has
	// no record of, which reads as still working.
	Status job.Status
	// Waiting names the ask a retained child is blocked on — "permission",
	// "question", "continue" — and is empty otherwise.
	Waiting string
	// Tools counts the child's tool rows in the parent's transcript; 0 means
	// the parent never watched this run, so the row says nothing about tools.
	Tools int
	// Created orders the working children; Started/Finished bound the elapsed
	// time. A zero Started means the job never ran, so it has no elapsed time.
	Created  time.Time
	Started  time.Time
	Finished time.Time
	// Summary is the outcome the child reported, Error the failure it died of.
	Summary string
	Error   string
	// Retained marks a child whose session this process still holds, and which
	// Enter can therefore open. A finished, released child has only its files.
	Retained bool
	// ResultPath is where the child's result was written — the answer Enter
	// gives when there is no session left to open.
	ResultPath string
}

// running reports a child that has not reached a terminal status.
func (a Agent) running() bool { return !a.Status.Terminal() }

// glyph is the row's state marker.
func (a Agent) glyph() string {
	switch a.Status {
	case job.StatusCompleted:
		return glyphDone
	case job.StatusCancelled:
		return glyphStopped
	case job.StatusFailed, job.StatusTimedOut:
		return glyphFailed
	default:
		if a.Waiting != "" {
			return glyphWaiting
		}
		return glyphRunning
	}
}

// SetTheme restyles the browser; the pane takes every color from it.
func (p *Pane) SetTheme(th components.Theme) {
	if p != nil {
		p.theme = th
	}
}

// Pane is the sub-agent browser. Mutated and rendered on the UI goroutine.
type Pane struct {
	theme components.Theme

	// snapshot re-pulls the session's children; onOpen shows one of them as
	// the current screen; onStop ends a running one; onClose fires once when
	// the pane stops being visible, so the shell can hand the keyboard back.
	snapshot func() []Agent
	onOpen   func(jobID string)
	onStop   func(jobID string) error
	onClose  func()

	// now is the clock the elapsed times run against, injectable so a test can
	// pin a row's age.
	now func() time.Time

	visible  bool
	viewport int // list rows available, measured by the last Draw

	// The standard machinery: the motion parser, the cursor it drives, and the
	// armed y/n confirmation for stop.
	motions browse.Motions
	cursor  browse.Cursor
	confirm browse.Confirm

	// notice is a one-keypress footer message: what a dead key should have
	// done, or where a released child's result went. The next key clears it.
	notice string
}

// New builds a hidden pane. The pane never reaches into the job manager or a
// session: every read and side effect goes back through these seams.
func New(
	theme components.Theme,
	snapshot func() []Agent,
	onOpen func(jobID string),
	onStop func(jobID string) error,
	onClose func(),
) *Pane {
	return &Pane{
		theme: theme, snapshot: snapshot,
		onOpen: onOpen, onStop: onStop, onClose: onClose,
		now: time.Now,
	}
}

// SetClock replaces the wall clock the elapsed times run against.
func (p *Pane) SetClock(now func() time.Time) {
	if p != nil && now != nil {
		p.now = now
	}
}

// Show opens the browser at the first row.
func (p *Pane) Show() {
	p.visible = true
	p.list() // prime the cursor's rows so selection works before the first Draw
	p.notice = ""
	p.confirm.Disarm()
	p.motions.Reset()
	p.cursor.Apply(browse.Motion{Op: browse.OpTop})
}

// Hide closes the browser, drops any pending confirmation and pending vim
// input, and notifies the shell so it can restore focus.
func (p *Pane) Hide() {
	if !p.visible {
		return
	}
	p.visible = false
	p.notice = ""
	p.confirm.Disarm()
	p.motions.Reset()
	if p.onClose != nil {
		p.onClose()
	}
}

// Visible reports whether the browser covers the screen.
func (p *Pane) Visible() bool { return p != nil && p.visible }

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
	// An armed confirmation gets the key first: y fires, n and Esc cancel, and
	// anything else withdraws the question and falls through.
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
		p.openSelected()
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
	switch {
	case e.Rune == ' ':
		p.openSelected()
	case e.HotkeyRune() == 'q':
		p.Hide()
	case e.HotkeyRune() == 'x':
		p.requestStop()
	default:
		p.notice = keys.Hints(keys.ScopeAgentList)
	}
}

// openSelected shows the selected child's session. Only a retained child has
// one: a finished child the session no longer holds is a directory on disk, so
// the pane says where its result went instead of pretending to open it.
func (p *Pane) openSelected() {
	a, ok := p.selected()
	if !ok {
		return
	}
	if !a.Retained {
		p.notice = releasedNotice(a)
		return
	}
	// The browser covers the screen it is about to hand over, so it closes
	// first — and that also gives the keyboard back through onClose.
	p.Hide()
	if p.onOpen != nil {
		p.onOpen(a.JobID)
	}
}

// releasedNotice names where a child that is no longer open left its answer.
func releasedNotice(a Agent) string {
	if a.ResultPath == "" {
		return "this agent's session is closed and it left no result on disk"
	}
	return "this agent's session is closed — its result is at " + a.ResultPath
}

// requestStop arms the stop confirmation for the selected child, capturing its
// id and title now: the question must fire on the row it named, not on wherever
// the cursor sits when y lands.
func (p *Pane) requestStop() {
	a, ok := p.selected()
	if !ok {
		return
	}
	if !a.running() {
		p.notice = "this agent already finished — nothing to stop"
		return
	}
	id, title := a.JobID, a.Title
	p.confirm.Arm(fmt.Sprintf("stop agent %q?", title), func() {
		if p.onStop != nil {
			_ = p.onStop(id) // the shell toasts errors
		}
	})
}

// selected returns the agent under the cursor, if any.
func (p *Pane) selected() (Agent, bool) {
	as := p.list()
	if len(as) == 0 {
		return Agent{}, false
	}
	return as[p.cursor.Selected()], true
}

// list re-reads the snapshot, orders it and re-teaches the cursor its rows.
// Draw calls it every frame: children finish under the pane, and the cursor
// must never point past the end.
func (p *Pane) list() []Agent {
	if p.snapshot == nil {
		return nil
	}
	as := order(p.snapshot())
	p.cursor.SetRows(len(as), nil)
	return as
}

// order puts the children still working on top, oldest first — that is the
// order they were started in, and the order the band shows them in — and the
// finished ones below, newest first, because history is read backwards.
func order(as []Agent) []Agent {
	out := append([]Agent(nil), as...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.running() != b.running() {
			return a.running()
		}
		if a.running() {
			return a.Created.Before(b.Created)
		}
		if !a.Finished.Equal(b.Finished) {
			return a.Finished.After(b.Finished)
		}
		return a.Created.After(b.Created)
	})
	return out
}

// Draw renders the whole screen: header, agent rows, footer.
func (p *Pane) Draw(ctx components.DrawContext) components.Surface {
	w, h := ctx.Max.Width, ctx.Max.Height
	if w <= 0 {
		w = 40
	}
	if h <= 0 {
		h = 24
	}
	as := p.list()
	p.viewport = max(h-chromeRows, 1)
	p.cursor.SetViewport(p.viewport)

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

	y := 0
	s.Print(1, y, layout.TruncateToWidth(header(as), w-2, ctx.Method), th.Warning, ctx.Method)
	y++

	if len(as) == 0 {
		s.Print(1, y, " no sub-agents in this session yet", th.Muted, ctx.Method)
		y++
	}
	ticking := false
	for i := range p.viewport {
		idx := p.cursor.Scroll() + i
		if idx >= len(as) {
			break
		}
		item := as[idx]
		ticking = ticking || (item.running() && !item.Started.IsZero())
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
	if ticking {
		// The elapsed times move on frames the pane asks for; nothing here
		// runs a clock of its own.
		ctx.WakeIn(tick)
	}

	// Footer: the catalog's own hint row, or the pending stop confirmation.
	hint := " " + keys.Hints(keys.ScopeAgentList)
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
	return s
}

// header counts the children still working against the whole history.
func header(as []Agent) string {
	live := 0
	for _, a := range as {
		if a.running() {
			live++
		}
	}
	return fmt.Sprintf(" Agents  %d running · %d total", live, len(as))
}

// row renders one child: glyph, title, the facts the parent recorded, and the
// one line the child came back with. The title is what gives way when the row
// is too narrow — the counts and the outcome are why the user opened the list.
func (p *Pane) row(a Agent, width int, method xui.WidthMethod) string {
	lead := a.glyph() + " "
	facts := ""
	if a.running() && a.Waiting != "" {
		facts += " · waiting: " + a.Waiting
	}
	facts += p.suffix(a)

	room := width - xui.StringWidth(lead+facts, method)
	head := lead + layout.EllipsizeToWidth(a.Title, max(room, titleFloor), method) + facts
	head = layout.TruncateToWidth(head, width, method)
	if line := summary(a); line != "" {
		tail := " · " + line
		head += layout.TruncateToWidth(tail, max(width-xui.StringWidth(head, method), 0), method)
	}
	return head
}

// suffix is the row's " · N tools · 1m 20s" tail. A run the parent never
// watched counts no tools and says so by omission; a job that never started has
// no elapsed time to report.
func (p *Pane) suffix(a Agent) string {
	out := ""
	if a.Tools > 0 {
		out += " · " + strconv.Itoa(a.Tools) + " tools"
	}
	if a.Started.IsZero() {
		return out
	}
	end := a.Finished
	if end.IsZero() {
		end = p.now()
	}
	if d := components.FormatDuration(end.Sub(a.Started)); d != "" {
		out += " · " + d
	}
	return out
}

// summary is the one line the row ends with: what the child reported, else what
// it died of, else nothing at all.
func summary(a Agent) string {
	line := a.Summary
	if line == "" {
		line = a.Error
	}
	return strings.Join(strings.Fields(line), " ")
}
