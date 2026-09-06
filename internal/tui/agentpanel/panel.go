// Package agentpanel renders the band of sub-agent rows that sits between the
// composer and the footer while a session has children: a first row for the
// parent session, one row per child, and the unselectable indicator rows that
// count what the three-row window hides.
//
// The panel knows nothing about jobs, controllers or views. It reads a
// snapshot through the rows seam on every Draw and pushes every side effect
// back out through Actions, so the wiring decides what opening or stopping a
// child means. The clock is injected too, which is what makes the 30-second
// lifecycle windows testable without sleeping.
//
// Motions, the cursor and the scroll window come from internal/tui/browse and
// the hint row from internal/tui/keys, as internal/tui/DESIGN.md requires; the
// panel owns only its rows and its actions.
package agentpanel

import (
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/browse"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

// State is what a child looks like right now.
type State int

const (
	// StateRunning is a child working on its turn.
	StateRunning State = iota
	// StateWaiting is a child blocked on a permission, question or continue
	// ask; Row.Waiting names which.
	StateWaiting
	// StateDone is a child that finished successfully. Its row leaves the
	// panel at once and arms the footer hint instead.
	StateDone
	// StateFailed is a child whose run ended in an error.
	StateFailed
	// StateStopped is a child the user stopped.
	StateStopped
)

// Row is one child as the wiring sees it right now. ID "" is never passed:
// the panel adds the main row itself.
type Row struct {
	ID string
	// Title is already formatted by the wiring, e.g.
	// "explore(find the config loader) · skills: a, b".
	Title   string
	State   State
	Waiting string // "permission" | "question" | "continue" when State == StateWaiting
	Tools   int
	Started time.Time
	Ended   time.Time // zero until terminal
}

// Actions are what the panel asks the wiring to do.
type Actions struct {
	Open  func(id string)       // Enter/Space/click: "" means the main row
	Stop  func(id string) error // x on a running or waiting row
	Leave func()                // Esc, or ↑ on the main row: focus goes back to the composer
}

const (
	// window is how long a failed or stopped row stays on screen, and how
	// long the footer hint outlives a success.
	window = 30 * time.Second
	// viewport is how many list rows the band shows at once. The indicator
	// rows sit outside it, which is what keeps the whole band at five rows.
	viewport = 3
	// tick is how often a running row redraws so its elapsed time moves.
	tick = time.Second
	// hintText is the footer's reminder that finished children are still
	// reachable after their rows are gone.
	hintText = "/agents to see agents"
	// mainTitle names the parent session's row.
	mainTitle = "main"
)

// The row glyphs: the first column says which session the screen is showing,
// the second what the child is doing.
const (
	glyphCurrent = "●"
	glyphOther   = "○"
	glyphRunning = "⟳"
	glyphWaiting = "⏸"
	glyphDone    = "✓"
	glyphFailed  = "✗"
	glyphStopped = "■"
	glyphUp      = "↑"
	glyphDown    = "↓"
)

// terminal reports whether the state is an end state.
func (s State) terminal() bool {
	return s == StateDone || s == StateFailed || s == StateStopped
}

// glyph is the state's marker in the row's second column.
func (s State) glyph() string {
	switch s {
	case StateWaiting:
		return glyphWaiting
	case StateDone:
		return glyphDone
	case StateFailed:
		return glyphFailed
	case StateStopped:
		return glyphStopped
	default:
		return glyphRunning
	}
}

// label is the word an ended row carries after its title; a running row says
// it with the glyph alone, and a waiting row names its ask instead.
func (s State) label() string {
	switch s {
	case StateFailed:
		return "failed"
	case StateStopped:
		return "stopped"
	default:
		return ""
	}
}

// memory is what the panel needs to remember about one ID between frames.
// It is deliberately tiny: an ID the seam stops returning is forgotten.
type memory struct {
	// terminal is when the panel first saw this row in an end state. It
	// anchors the 30-second window when the seam gives no Ended, and freezes
	// the elapsed time for the same case.
	terminal time.Time
	// dismissed marks a failed or stopped row cleared early with x.
	dismissed bool
}

// view is the layout one build produced: the children that survived the
// lifecycle filter, plus where the three-row window sits over them.
type view struct {
	children []Row
	scroll   int
	visible  int // list rows drawn, main included
	above    int // list rows hidden above the window
	below    int // list rows hidden below the window
}

// total counts the list rows: the main row plus the children.
func (v view) total() int { return len(v.children) + 1 }

// height is how many rows the band wants: nothing when no child survived the
// lifecycle filter, else the visible list rows plus an indicator row for each
// side that hides something.
func (v view) height() int {
	if len(v.children) == 0 {
		return 0
	}
	h := v.visible
	if v.above > 0 {
		h++
	}
	if v.below > 0 {
		h++
	}
	return h
}

// rowAt maps a band-relative row onto a list-row index. The indicator rows
// answer false: they are chrome, and a click on one does nothing.
func (v view) rowAt(y int) (int, bool) {
	if v.above > 0 {
		if y == 0 {
			return 0, false
		}
		y--
	}
	if y < 0 || y >= v.visible {
		return 0, false
	}
	return v.scroll + y, true
}

// id maps a list-row index onto the ID the actions take; index 0 is main,
// whose ID is the empty string.
func (v view) id(i int) string {
	if i <= 0 || i > len(v.children) {
		return ""
	}
	return v.children[i-1].ID
}

// Panel is the sub-agent band. Mutated and rendered on the UI goroutine.
type Panel struct {
	theme   components.Theme
	rows    func() []Row
	clock   func() time.Time
	actions Actions

	// current is the ID of the session the screen is showing; "" is the
	// parent. It only decides which row wears ● and where Focus lands.
	current string
	focused bool

	// The standard machinery: the motion parser and the cursor it drives.
	motions browse.Motions
	cursor  browse.Cursor

	// notice is a one-keypress message drawn over the band's last row — a
	// stop that failed, or the keys a dead key should have been. The next
	// key clears it. It takes a row rather than adding one, so a refused
	// keypress never makes the composer jump.
	notice string

	// seen is the per-ID memory, pruned to what the seam still returns.
	seen map[string]*memory

	// hintUntil is when the footer's success hint expires.
	hintUntil time.Time
}

// New builds a panel over the rows seam. The panel never reaches into the job
// manager or the controller: every read comes through rows, every side effect
// goes back through actions, and every deadline is measured on now.
func New(theme components.Theme, rows func() []Row, now func() time.Time, actions Actions) *Panel {
	if now == nil {
		now = time.Now
	}
	return &Panel{
		theme:   theme,
		rows:    rows,
		clock:   now,
		actions: actions,
		seen:    map[string]*memory{},
	}
}

// SetCurrent names the session the screen is showing — "" for the parent —
// so that row wears ● and Focus lands on it.
func (p *Panel) SetCurrent(id string) {
	if p != nil {
		p.current = id
	}
}

// SetTheme swaps the palette; the panel takes every color from it.
func (p *Panel) SetTheme(t components.Theme) {
	if p != nil {
		p.theme = t
	}
}

// Visible reports whether the band has anything to show: at least one child
// row survived the lifecycle filter. The main row alone is not a panel.
func (p *Panel) Visible() bool {
	if p == nil {
		return false
	}
	return len(p.build().children) > 0
}

// Height is how many rows the band wants: zero when it is not visible, else
// the visible list rows (at most three, the main row included) plus an
// indicator row for each side that hides something.
func (p *Panel) Height() int {
	if p == nil {
		return 0
	}
	return p.build().height()
}

// Focus puts the keyboard in the band, with the cursor on the row of the
// session the screen is showing.
func (p *Panel) Focus() {
	if p == nil {
		return
	}
	p.focused = true
	p.notice = ""
	p.motions.Reset()
	v := p.build()
	p.cursor.Select(indexOf(v, p.current))
}

// Blur hands the keyboard back and drops pending motions and the notice, so
// a half-typed count never survives a trip through the composer.
func (p *Panel) Blur() {
	if p == nil {
		return
	}
	p.focused = false
	p.notice = ""
	p.motions.Reset()
}

// Focused reports whether keys reach the band.
func (p *Panel) Focused() bool { return p != nil && p.focused }

// Hint is the footer's reminder that a child finished: live for 30 seconds
// from the moment the panel first saw a success, because the row itself
// leaves at once.
func (p *Panel) Hint() (string, bool) {
	if p == nil {
		return "", false
	}
	p.build() // a success that has not been drawn yet still arms the hint
	if p.clock().Before(p.hintUntil) {
		return hintText, true
	}
	return "", false
}

// HandleEvent drives the band. Keys arrive only while it is focused; mouse
// events arrive whenever the pointer is over it, with coordinates the wiring
// has already made panel-relative.
func (p *Panel) HandleEvent(ctx *components.EventContext, ev xui.Event) bool {
	if p == nil {
		return false
	}
	switch e := ev.(type) {
	case xui.MouseEvent:
		return p.handleMouse(ctx, e)
	case xui.KeyEvent:
		if !p.focused {
			return false
		}
		if !e.Press {
			return true
		}
		p.handleKey(e)
		consume(ctx)
		return true
	default:
		return false
	}
}

// handleMouse selects and opens on a left press, scrolls the window on the
// wheel, and lets everything else through. A press on an indicator is eaten
// without acting: the row is chrome, not a target.
func (p *Panel) handleMouse(ctx *components.EventContext, e xui.MouseEvent) bool {
	v := p.build()
	if len(v.children) == 0 {
		return false
	}
	switch e.Button {
	case xui.MouseWheelUp, xui.MouseWheelDown:
		if !p.cursor.Wheel(e) {
			return false
		}
		consume(ctx)
		return true
	case xui.MouseLeft:
		if e.Action != xui.MousePress {
			return false
		}
		if idx, ok := v.rowAt(e.Y); ok {
			p.cursor.Select(idx)
			p.open(v.id(idx))
		}
		consume(ctx)
		return true
	default:
		return false
	}
}

// handleKey runs one keypress through the dialect first, then the band's own
// keys. A key the band cannot use answers with the catalog's hint row rather
// than dying silently.
func (p *Panel) handleKey(e xui.KeyEvent) {
	p.notice = ""
	v := p.build()
	if m, ok := p.motions.Key(e); ok {
		// A step up from the main row is how the user gets back to the
		// composer; there is nothing above it to move onto anyway.
		if m.Op == browse.OpStep && m.N < 0 && p.cursor.Selected() == 0 {
			p.leave()
			return
		}
		p.cursor.Apply(m)
		return
	}
	switch e.Code {
	case xui.KeyEscape:
		p.leave()
	case xui.KeyEnter:
		p.open(v.id(p.cursor.Selected()))
	case xui.KeyRune:
		p.handleRune(e, v)
	default:
		p.deadKey()
	}
}

// handleRune covers the band's own letters; the motion dialect (j/k, counts,
// gg/G, Ctrl+U/D) is already claimed by the shared parser.
func (p *Panel) handleRune(e xui.KeyEvent, v view) {
	if e.Mods != 0 {
		p.deadKey()
		return
	}
	switch {
	case e.Rune == ' ':
		p.open(v.id(p.cursor.Selected()))
	case e.HotkeyRune() == 'x':
		p.stopOrDismiss(v)
	default:
		p.deadKey()
	}
}

// stopOrDismiss is the whole of x: a live child is stopped through the seam,
// an ended row is cleared before its window runs out, and main is neither.
func (p *Panel) stopOrDismiss(v view) {
	idx := p.cursor.Selected()
	if idx <= 0 || idx > len(v.children) {
		p.notice = "x stops a child agent — main is this session"
		return
	}
	r := v.children[idx-1]
	switch r.State {
	case StateRunning, StateWaiting:
		if p.actions.Stop == nil {
			return
		}
		if err := p.actions.Stop(r.ID); err != nil {
			p.notice = "stop failed: " + err.Error()
		}
	default:
		if m := p.seen[r.ID]; m != nil {
			m.dismissed = true
		}
		p.build() // the row is gone; re-teach the cursor before the next key
	}
}

// deadKey answers a key the band has no use for with the keys that work,
// read from the catalog so the notice cannot drift from the help screen.
func (p *Panel) deadKey() { p.notice = keys.Hints(keys.ScopeAgents) }

// open hands an ID to the wiring; "" is the main row.
func (p *Panel) open(id string) {
	if p.actions.Open != nil {
		p.actions.Open(id)
	}
}

// leave gives the keyboard back. The panel blurs itself first, so a wiring
// that forgets to move focus cannot trap the keyboard in the band.
func (p *Panel) leave() {
	p.Blur()
	if p.actions.Leave != nil {
		p.actions.Leave()
	}
}

// indexOf finds the list row an ID sits on; an unknown ID lands on main.
func indexOf(v view, id string) int {
	if id == "" {
		return 0
	}
	for i, r := range v.children {
		if r.ID == id {
			return i + 1
		}
	}
	return 0
}

// build re-reads the seam, applies the lifecycle filter, re-teaches the
// cursor its rows and measures the window. Every entry point calls it, so
// the panel always answers about the rows that exist right now.
func (p *Panel) build() view {
	now := p.clock()
	src := []Row(nil)
	if p.rows != nil {
		src = p.rows()
	}
	if p.seen == nil {
		p.seen = map[string]*memory{}
	}

	kept := make([]Row, 0, len(src))
	live := make(map[string]bool, len(src))
	for _, r := range src {
		if r.ID == "" {
			continue // the panel owns the main row; a seam row cannot claim it
		}
		live[r.ID] = true
		if row, ok := p.admit(r, now); ok {
			kept = append(kept, row)
		}
	}
	for id := range p.seen {
		if !live[id] {
			delete(p.seen, id)
		}
	}

	v := view{children: kept}
	p.cursor.SetRows(v.total(), nil)
	v.visible = min(viewport, v.total())
	p.cursor.SetViewport(v.visible)
	v.scroll = p.cursor.Scroll()
	v.above = v.scroll
	v.below = max(v.total()-v.scroll-v.visible, 0)
	return v
}

// admit runs one seam row through the lifecycle: a success leaves at once and
// arms the footer hint, a failure or a stop stays for its window unless x
// cleared it, and anything live stays. The returned row carries a resolved
// Ended so the rest of the panel needs no clock of its own.
func (p *Panel) admit(r Row, now time.Time) (Row, bool) {
	m := p.seen[r.ID]
	if m == nil {
		m = &memory{}
		p.seen[r.ID] = m
	}
	if !r.State.terminal() {
		*m = memory{} // a row that went live again starts its windows over
		return r, true
	}
	if m.terminal.IsZero() {
		m.terminal = now
	}
	if r.Ended.IsZero() {
		r.Ended = m.terminal // freeze elapsed and anchor the window anyway
	}
	if r.State == StateDone {
		if until := m.terminal.Add(window); until.After(p.hintUntil) {
			p.hintUntil = until
		}
		return Row{}, false
	}
	if m.dismissed || !now.Before(r.Ended.Add(window)) {
		return Row{}, false
	}
	return r, true
}

// consume marks an event handled, tolerating the nil context tests pass.
func consume(ctx *components.EventContext) {
	if ctx != nil {
		ctx.ConsumeAndRedraw()
	}
}
