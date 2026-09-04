package chat

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/components/text"
	"github.com/alvnukov/cozyphi/internal/debuglog"
)

// ChatInput is a composer in the opencode prompt style: a left ┃ bar in the
// posture color wraps a backgroundElement panel, the meta row sits inside the
// frame bottom, a ╹▀ tail fades the frame into the terminal, and a hints row
// (cwd left, usage/keymap right) sits below.
//
// Layout (minBodyRows=3 → total height 8; +1 when PendingSkills set):
//
//	┃                                                ┃
//	┃ Skills: building-plugins                       ┃
//	┃█                                               ┃
//	┃ ⏵⏵ build · model                              ┃
//	╹▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
//	 ~/cwd                                5% of 128k
type ChatInput struct {
	// Value is the current editor text (may contain newlines).
	Value string
	// Cursor is a byte offset into Value.
	Cursor int

	MinBodyRows int // default 3
	MaxBodyRows int // default 12; height grows with content up to this

	// AgentLabel is the posture lead ("⏵⏵ build") in the meta row; its style
	// also colors the left ┃ bar and the ╹ tail.
	AgentLabel layout.BorderLabel
	// ModelLabel follows the posture lead in the meta row after a muted " · ".
	ModelLabel string
	// HintsLeft is the muted cwd text on the hints row below the frame.
	HintsLeft string
	// HintsRight is the usage span group right-aligned on the hints row;
	// empty falls back to the keymap hints ("tab mode  ^k commands").
	HintsRight []components.Span
	// Placeholder renders muted inside the frame while Value is empty
	// (opencode: "Ask anything..." / "Run a command...").
	Placeholder string

	TextStyle      xui.Style
	CursorStyle    xui.Style // visual block when terminal cursor unavailable
	UseBlockCursor bool      // paint reverse cell in addition to terminal cursor

	// Theme styles the chrome: BackgroundElement panel, Muted hint text, and
	// the pending-skills chip row (Muted label, Success names).
	Theme components.Theme

	// PendingSkills are skill names shown inside the frame as the first
	// content row: "Skills: name1 name2".
	PendingSkills []string

	// OnSubmit is called when Enter is pressed (without modifiers).
	OnSubmit func(text string)
	// OnChange is called after Value mutates.
	OnChange func(text string)
	// OnPendingSkillsChange is called after PendingSkills mutates.
	OnPendingSkillsChange func(skills []string)
	// OnMentionChange is called after Value or Cursor changes that may
	// activate/deactivate an @-file mention. active is false when none.
	OnMentionChange func(active bool, query string)
	// OnSlashChange is called after Value or Cursor changes that may
	// activate/deactivate a leading /command. active is false when none.
	OnSlashChange func(active bool, query string)
	// OnSlashArgChange is called after Value or Cursor changes that may
	// move the cursor into an argument of a leading /command. args holds the
	// arguments completed before the cursor, so a completer knows which
	// argument it is offering values for. active is false when the cursor is
	// not in an argument position.
	OnSlashArgChange func(active bool, name string, args []string, partial string)

	// MentionOpen is set by the editor while the @-file picker is visible.
	// When true, Up/Down/Tab/Enter are left unconsumed so the picker can
	// handle navigation (focus stays on the composer for typing).
	MentionOpen bool
	// SlashOpen is set while the /command picker is visible (same nav deferral).
	SlashOpen bool
	// VoiceMode is set by the composer while the voice dialog mode is on.
	// When true, a plain Space and a plain Enter are left unconsumed so the
	// mode's key handling sees them — dispatch delivers to the focused input
	// before the pane, so without the deferral the space is typed and the
	// message sent before the mode gets a look. An open picker takes the
	// space back (it belongs in the query), and everything else types as
	// usual.
	VoiceMode bool

	// History recalls past submissions on Up from the text origin and Down
	// from the end (opencode prompt.history.previous/next); nil keeps the
	// plain caret movement.
	History Recaller

	// OnCopy sends text to the system clipboard; copy/cut chords and a
	// finished mouse drag over an active selection go through it (copy-on-
	// select, as the transcript does). nil leaves chords unconsumed so the
	// legacy routing (quit / transcript copy) keeps working.
	OnCopy func(text string) bool

	// selAnchor is where the selection began; the selection spans
	// selAnchor..Cursor in either direction. hasSel with anchor == cursor is
	// a collapsed (empty) selection.
	selAnchor int
	hasSel    bool
	// dragging is true between a mouse press and its release inside the editor.
	dragging bool
	// rows/rowsW/rowsScroll cache the last drawn layout so mouse events can
	// map screen cells back to byte offsets without re-deriving geometry.
	rowsW      int
	rows       []visRow
	rowsScroll int

	// dumpNextDraw is set on paste/insert when COZYPHI_DEBUG=1.
	dumpNextDraw bool

	// search is the reverse-i-search mode state (search.go), owned by value.
	search search
}

// Recaller walks the composer's prompt history: Prev steps to the previous
// submission, Next steps back toward the draft, and Search lists the entries
// containing a query, newest first. history.Store implements it; the seam
// keeps the widget package free of the storage import.
type Recaller interface {
	Prev(draft string) (string, bool)
	Next(draft string) (string, bool)
	Search(query string) []string
}

// recall applies a history walk — Up claims the key only while the caret is
// on the first line, Down only while it is on the last, so multiline drafts
// keep their caret movement — and reports whether the value was replaced.
// The caret lands at the end of the recalled text, ready for editing.
func (c *ChatInput) recall(code xui.KeyCode) bool {
	if c.History == nil {
		return false
	}
	var (
		text string
		ok   bool
	)
	switch code {
	case xui.KeyUp:
		if lineStart(c.Value, c.Cursor) != 0 {
			return false
		}
		text, ok = c.History.Prev(c.Value)
	case xui.KeyDown:
		if lineEnd(c.Value, c.Cursor) != len(c.Value) {
			return false
		}
		text, ok = c.History.Next(c.Value)
	default:
		return false
	}
	if !ok {
		return false
	}
	c.Value = text
	c.Cursor = len(text)
	c.ClearSelection()
	c.notifyChange()
	return true
}

func (c *ChatInput) completerOpen() bool {
	return c.MentionOpen || c.SlashOpen
}

func (c *ChatInput) bodyRows(width int, method xui.WidthMethod) int {
	minR := c.minBodyRows()
	maxR := c.MaxBodyRows
	if maxR < 1 {
		maxR = 12
	}
	if maxR < minR {
		maxR = minR
	}
	innerW := width - 5 // bar + paddingLeft 2 + paddingRight 2
	innerW = max(innerW, 1)
	n := len(layoutEditor(c.Value, innerW, method))
	n = max(n, 1)
	n = max(n, minR)
	n = min(n, maxR)
	return n
}

// minBodyRows is the configured editor-row floor (zero → 3).
func (c *ChatInput) minBodyRows() int {
	if c.MinBodyRows < 1 {
		return 3
	}
	return c.MinBodyRows
}

// MinHeight is the smallest total height at the minimum body rows — the one
// number layout code clamps short screens against, instead of re-deriving
// the floor at every call site.
func (c *ChatInput) MinHeight() int {
	return c.pendingSkillsHeight() + c.minBodyRows() + 5
}

// PreferredHeight returns total height (pad + skills/body + gap + meta + tail
// + hints), growing with content up to MaxBodyRows so the composer cannot
// expand forever.
func (c *ChatInput) PreferredHeight(width int, method xui.WidthMethod) int {
	return c.pendingSkillsHeight() + c.bodyRows(width, method) + 5
}

func (c *ChatInput) pendingSkillsHeight() int {
	if len(c.PendingSkills) == 0 {
		return 0
	}
	return 1
}

// AddPendingSkill appends name if not already pending.
func (c *ChatInput) AddPendingSkill(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if slices.Contains(c.PendingSkills, name) {
		return
	}
	c.PendingSkills = append(c.PendingSkills, name)
	c.notifyPendingSkills()
}

// PopPendingSkill removes the last pending skill. Returns false if none.
func (c *ChatInput) PopPendingSkill() bool {
	if len(c.PendingSkills) == 0 {
		return false
	}
	c.PendingSkills = c.PendingSkills[:len(c.PendingSkills)-1]
	c.notifyPendingSkills()
	return true
}

// ClearPendingSkills removes all pending skills.
func (c *ChatInput) ClearPendingSkills() {
	if len(c.PendingSkills) == 0 {
		return
	}
	c.PendingSkills = nil
	c.notifyPendingSkills()
}

func (c *ChatInput) notifyPendingSkills() {
	if c.OnPendingSkillsChange != nil {
		c.OnPendingSkillsChange(c.PendingSkills)
	}
}

func (c *ChatInput) clampCursor() {
	if c.Cursor < 0 {
		c.Cursor = 0
	}
	if c.Cursor > len(c.Value) {
		c.Cursor = len(c.Value)
	}
}

// PointerShape marks the composer as editable text.
func (*ChatInput) PointerShape(_, _ int) string { return components.ShapeText }

// Handle edits the composer value: typing, navigation, selection, submit on
// Enter, clipboard chords, mouse caret/selection, and pending-skill backspace
// removal.
func (c *ChatInput) Handle(ctx *components.EventContext, ev xui.Event) {
	switch e := ev.(type) {
	case xui.KeyEvent:
		if !e.Press {
			return
		}
		c.clampCursor()
		// Reverse-i-search owns the keyboard while it is on: the mode-ending
		// keys, the query edits and Enter land here, inside the focused
		// widget, so the pane ladder never sees them mid-search.
		if c.search.active && c.handleSearchKey(ctx, e) {
			return
		}
		if c.handleChord(ctx, e) {
			return
		}
		switch e.Code {
		case xui.KeyEnter:
			// Let the @-file / slash picker accept / consume Enter, and the
			// voice dialog mode close its segment with it. Modified Enter
			// still inserts a newline below, in the mode as much as out of it.
			if c.completerOpen() || (c.VoiceMode && e.Mods == 0) {
				return
			}
			// Shift+Enter / Alt+Enter insert newline; bare Enter submits.
			if e.Mods.Has(xui.ModShift) || e.Mods.Has(xui.ModAlt) || e.Mods.Has(xui.ModCtrl) {
				c.insert("\n")
				ctx.ConsumeAndRedraw()
				return
			}
			if c.OnSubmit != nil {
				c.OnSubmit(c.Value)
			}
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyBackspace:
			c.backspace(e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt))
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyDelete:
			c.deleteForward(e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt))
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyLeft:
			c.arrowLeft(e)
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyRight:
			c.arrowRight(e)
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyHome:
			if rows := c.editRows(); rows != nil {
				row, _ := offsetToRowCol(rows, c.Cursor, c.rowsW)
				c.moveTo(rows[row].start, e.Mods.Has(xui.ModShift))
			} else {
				c.moveTo(lineStart(c.Value, c.Cursor), e.Mods.Has(xui.ModShift))
			}
			c.notifyCompleters()
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyEnd:
			if rows := c.editRows(); rows != nil {
				row, _ := offsetToRowCol(rows, c.Cursor, c.rowsW)
				c.moveTo(rows[row].end, e.Mods.Has(xui.ModShift))
			} else {
				c.moveTo(lineEnd(c.Value, c.Cursor), e.Mods.Has(xui.ModShift))
			}
			c.notifyCompleters()
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyUp:
			if c.completerOpen() {
				return
			}
			if rows := c.editRows(); rows != nil {
				// History only claims Up from the first visual row; wrapped
				// rows above stay caret moves.
				row, _ := offsetToRowCol(rows, c.Cursor, c.rowsW)
				if row > 0 {
					// Vertical movement is caret-only: it must not re-evaluate the
					// slash/mention pickers, or a dismissed picker re-opens just
					// because the caret moved back into a leading "/" token.
					c.moveVisual(-1, e.Mods.Has(xui.ModShift))
					ctx.ConsumeAndRedraw()
					return
				}
			}
			if c.recall(xui.KeyUp) {
				ctx.ConsumeAndRedraw()
				return
			}
			c.moveVert(-1)
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyDown:
			if c.completerOpen() {
				return
			}
			if rows := c.editRows(); rows != nil {
				row, col := offsetToRowCol(rows, c.Cursor, c.rowsW)
				atEnd := row == len(rows)-1 && col >= rows[row].width
				if !atEnd {
					c.moveVisual(1, e.Mods.Has(xui.ModShift))
					ctx.ConsumeAndRedraw()
					return
				}
			}
			if c.recall(xui.KeyDown) {
				ctx.ConsumeAndRedraw()
				return
			}
			c.moveVert(1)
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyTab:
			if c.completerOpen() {
				return
			}
			return
		case xui.KeyRune:
			if e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt) || e.Mods.Has(xui.ModSuper) {
				return
			}
			// A plain Space belongs to the voice dialog mode; an open
			// picker takes it back, because its query is typed here.
			if c.VoiceMode && !c.completerOpen() && e.Rune == ' ' && e.Mods == 0 {
				return
			}
			if e.Rune >= 0x20 || e.Rune == '\t' {
				c.insert(string(e.Rune))
				ctx.ConsumeAndRedraw()
			}
			return
		}
	case xui.MouseEvent:
		// A click is not a search key: end the mode with the match in the
		// buffer, so the caret lands in real text instead of preview rows.
		if c.search.active {
			c.searchAccept()
		}
		c.handleMouse(ctx, e)
	case xui.PasteEvent:
		// Pasting ends the search the same way: the text belongs in the
		// buffer, not in the query.
		if c.search.active {
			c.searchAccept()
		}
		debuglog.Logf("chat paste raw bytes=%d", len(e.Text))
		debuglog.DumpRunes("chat paste raw", e.Text)
		c.insert(e.Text)
		debuglog.DumpRunes("chat value after paste", c.Value)
		debuglog.Logf("chat cursor=%d", c.Cursor)
		ctx.ConsumeAndRedraw()
	}
}

// handleChord serves clipboard/selection chords before the plain-key switch:
// Ctrl/Cmd+A selects all, Ctrl/Cmd+C (any shift) copies the selection,
// Ctrl/Cmd+X cuts it, and Ctrl/Cmd+U discards the line before the caret. It
// reports whether the event was consumed; chords that do not apply bubble on
// unchanged (Ctrl+C without a selection still quits).
func (c *ChatInput) handleChord(ctx *components.EventContext, e xui.KeyEvent) bool {
	if e.Code != xui.KeyRune {
		return false
	}
	// While reverse-i-search is on, the editing chords have no buffer to
	// act on — the body is a preview — so they bubble on unchanged; the
	// pane-level search chords (Ctrl+R/Ctrl+S/Ctrl+G) ride the same fall-
	// through to the ladder.
	if c.search.active {
		return false
	}
	switch {
	case isChord(e, 'a', 'A'):
		c.SelectAll()
		ctx.ConsumeAndRedraw()
		return true
	case isChord(e, 'u', 'U'):
		c.killToLineStart()
		ctx.ConsumeAndRedraw()
		return true
	case isChord(e, 'c', 'C'), isChord(e, 'x', 'X'):
		if !c.HasSelection() || c.OnCopy == nil {
			return false
		}
		if !c.OnCopy(c.SelectedText()) {
			return false // clipboard failed: keep legacy routing
		}
		if isChord(e, 'x', 'X') {
			c.deleteSelection()
		}
		ctx.ConsumeAndRedraw()
		return true
	}
	return false
}

// isChord reports whether e is Ctrl/Cmd+key — the one encoding of "chord"
// shared by handleChord and AcceptCopyKey so the App quit path and the
// widget can never disagree about what counts as copy.
func isChord(e xui.KeyEvent, keys ...rune) bool {
	if !e.Press || e.Code != xui.KeyRune {
		return false
	}
	if !e.Mods.Has(xui.ModCtrl) && !e.Mods.Has(xui.ModSuper) {
		return false
	}
	return slices.Contains(keys, e.HotkeyRune())
}

// AcceptCopyKey reports whether the composer claims a Ctrl/Cmd+C press —
// an active selection turns it into copy instead of an app quit. Shift is
// irrelevant: xui's CtrlC() does not exclude it, so Ctrl+Shift+C would
// otherwise quit the app before handleChord could copy. The App runtime
// consults this before its built-in Ctrl+C exit.
func (c *ChatInput) AcceptCopyKey(e xui.KeyEvent) bool {
	return isChord(e, 'c', 'C') && c.HasSelection() && c.OnCopy != nil
}

// handleMouse maps a left-button press inside the editor body to a caret
// move and a press-drag-release to a selection. Coordinates are surface-local
// (the App hit-test delivers local coordinates to the pressed widget); drags
// that leave the editor clamp to its edge rows, which covers selecting toward
// the frame borders. Motion/drag only count with the button held: ?1003
// all-motion tracking also delivers buttonless hovers, which must never
// mutate the caret — including while a stale drag lingers after its release
// landed on another widget.
func (c *ChatInput) handleMouse(ctx *components.EventContext, e xui.MouseEvent) {
	left := e.Button == xui.MouseLeft
	if !left && !c.dragging {
		return
	}
	switch e.Action {
	case xui.MousePress:
		if !left {
			return
		}
		c.dragging = true
		c.hasSel = false
		c.selAnchor = c.pointOffset(e.X, e.Y)
		c.Cursor = c.selAnchor
		c.notifyCompleters()
		ctx.ConsumeAndRedraw()
	case xui.MouseDrag, xui.MouseMotion:
		if !c.dragging || !left {
			return
		}
		c.Cursor = c.pointOffset(e.X, e.Y)
		c.hasSel = c.Cursor != c.selAnchor
		ctx.ConsumeAndRedraw()
	case xui.MouseRelease:
		if !c.dragging {
			return
		}
		c.dragging = false
		c.Cursor = c.pointOffset(e.X, e.Y)
		c.hasSel = c.Cursor != c.selAnchor
		if c.hasSel && c.OnCopy != nil {
			// Copy-on-select mirrors the transcript: a finished drag lands in
			// the clipboard right away; the wired callback toasts the result.
			c.OnCopy(c.SelectedText())
		}
		ctx.ConsumeAndRedraw()
	}
}

// pointOffset maps a surface-local cell to a byte offset using the cached
// draw layout: editor rows sit below the skills chip, offset by the scroll.
func (c *ChatInput) pointOffset(x, y int) int {
	rows := c.rows
	if len(rows) == 0 {
		return c.Cursor
	}
	row := (y - 1 - c.pendingSkillsHeight()) + c.rowsScroll
	row = max(row, 0)
	row = min(row, len(rows)-1)
	col := x - 3 // bar + paddingLeft 2
	col = max(col, 0)
	col = min(col, c.rowsW)
	return rowColToOffset(rows, row, col)
}

// moveTo moves the caret, extending the selection from its anchor on shift
// or dropping it otherwise. Vertical callers skip notifyCompleters on
// purpose (dismissed pickers must not reopen).
func (c *ChatInput) moveTo(off int, extend bool) {
	c.clampCursor()
	off = max(off, 0)
	off = min(off, len(c.Value))
	if extend {
		// Arm the anchor only when the caret actually moves: a no-op
		// Shift+arrow at a boundary must not leave a phantom collapsed
		// selection that later swallows one Backspace.
		if !c.hasSel && off != c.Cursor {
			c.selAnchor = c.Cursor
			c.hasSel = true
		}
	} else {
		c.hasSel = false
		c.selAnchor = 0
	}
	c.Cursor = off
}

func (c *ChatInput) arrowLeft(e xui.KeyEvent) {
	var off int
	if e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt) {
		off = text.PrevWordStart(c.Value, c.Cursor)
	} else {
		off = c.Cursor
		if off > 0 {
			_, size := utf8.DecodeLastRuneInString(c.Value[:off])
			off -= size
		}
	}
	c.moveTo(off, e.Mods.Has(xui.ModShift))
	c.notifyCompleters()
}

func (c *ChatInput) arrowRight(e xui.KeyEvent) {
	var off int
	if e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt) {
		off = text.NextWordEnd(c.Value, c.Cursor)
	} else {
		off = c.Cursor
		if off < len(c.Value) {
			_, size := utf8.DecodeRuneInString(c.Value[off:])
			off += size
		}
	}
	c.moveTo(off, e.Mods.Has(xui.ModShift))
	c.notifyCompleters()
}

// moveVisual moves the caret one wrapped row up/down keeping the column.
// Falls back silently when no layout is cached (widget not drawn yet).
func (c *ChatInput) moveVisual(delta int, extend bool) {
	rows := c.editRows()
	if rows == nil {
		return
	}
	row, col := offsetToRowCol(rows, c.Cursor, c.rowsW)
	nrow := row + delta
	if nrow < 0 || nrow >= len(rows) {
		return
	}
	c.moveTo(rowColToOffset(rows, nrow, col), extend)
}

func (c *ChatInput) editRows() []visRow {
	if c.rowsW <= 0 || len(c.rows) == 0 {
		return nil
	}
	return c.rows
}

// HasSelection reports whether a non-empty selection is active.
func (c *ChatInput) HasSelection() bool {
	if !c.hasSel {
		return false
	}
	a, b := c.selectionRange()
	return b > a
}

// SelectedText returns the selected value slice ("" when none).
func (c *ChatInput) SelectedText() string {
	a, b := c.selectionRange()
	if b <= a {
		return ""
	}
	return c.Value[a:b]
}

// SetSelection selects [start,end) and places the caret at end.
func (c *ChatInput) SetSelection(start, end int) {
	c.clampCursor()
	start = max(start, 0)
	end = min(end, len(c.Value))
	if start > end {
		start, end = end, start
	}
	c.selAnchor = start
	c.Cursor = end
	c.hasSel = true
}

// SelectAll selects the whole value.
func (c *ChatInput) SelectAll() {
	c.SetSelection(0, len(c.Value))
}

// ClearSelection drops any selection, keeping the caret.
func (c *ChatInput) ClearSelection() {
	c.hasSel = false
	c.selAnchor = 0
}

// selectionRange returns the selection in reading order.
func (c *ChatInput) selectionRange() (int, int) {
	if !c.hasSel {
		return c.Cursor, c.Cursor
	}
	a, b := c.selAnchor, c.Cursor
	if a > b {
		a, b = b, a
	}
	return a, b
}

func (c *ChatInput) backspace(word bool) {
	if c.HasSelection() {
		deleted := c.SelectedText()
		c.deleteSelection()
		debuglog.Logf("chat backspace selection deleted=%q cursor=%d", deleted, c.Cursor)
		return
	}
	from := c.Cursor
	if word {
		from = text.PrevWordStart(c.Value, c.Cursor)
		// Consume the whitespace gap before the word too (VS Code behavior):
		// deleting "two" out of "one two" yields "one", not "one ".
		from = text.SkipLeftWhile(c.Value, from, unicode.IsSpace)
	} else if from > 0 {
		_, size := utf8.DecodeLastRuneInString(c.Value[:from])
		from -= size
	}
	if from < c.Cursor {
		c.deleteRange(from, c.Cursor)
		return
	}
	c.PopPendingSkill()
}

func (c *ChatInput) deleteForward(word bool) {
	if c.HasSelection() {
		c.deleteSelection()
		return
	}
	to := c.Cursor
	if word {
		to = text.NextWordEnd(c.Value, c.Cursor)
	} else if to < len(c.Value) {
		_, size := utf8.DecodeRuneInString(c.Value[to:])
		to += size
	}
	if to > c.Cursor {
		c.deleteRange(c.Cursor, to)
	}
}

// killToLineStart discards the text between the start of the current line and
// the caret, readline's Ctrl+U. The line, not the whole message: a composer
// holds a multi-line draft and has no undo, so one chord must not be able to
// wipe work the caret is nowhere near. On a single-line draft with the caret
// at its end — where the chord is nearly always pressed — that is the whole
// message anyway. A selection wins over the line, the way Backspace treats
// it, and at the start of a line there is nothing to discard.
func (c *ChatInput) killToLineStart() {
	if c.HasSelection() {
		c.deleteSelection()
		return
	}
	c.deleteRange(lineStart(c.Value, c.Cursor), c.Cursor)
}

func (c *ChatInput) deleteSelection() {
	if !c.HasSelection() {
		return
	}
	a, b := c.selectionRange()
	c.hasSel = false
	c.selAnchor = 0
	c.deleteRange(a, b)
}

func (c *ChatInput) deleteRange(from, to int) {
	from = max(from, 0)
	to = min(to, len(c.Value))
	if from >= to {
		return
	}
	deleted := c.Value[from:to]
	c.Value = c.Value[:from] + c.Value[to:]
	c.Cursor = from
	c.hasSel = false
	c.selAnchor = 0
	c.notifyChange()
	debuglog.Logf("chat delete range [%d,%d) deleted=%q", from, to, deleted)
	if debuglog.Enabled() {
		c.dumpNextDraw = true
	}
}

func (c *ChatInput) insert(s string) {
	before := s
	s = sanitizeComposerText(s)
	if s != before {
		debuglog.Logf("chat sanitize changed text bytes %d -> %d", len(before), len(s))
	}
	if s == "" {
		return
	}
	c.clampCursor()
	if c.HasSelection() {
		c.deleteSelection()
	}
	c.Value = c.Value[:c.Cursor] + s + c.Value[c.Cursor:]
	c.Cursor += len(s)
	if debuglog.Enabled() {
		c.dumpNextDraw = true
	}
	c.notifyChange()
}

// sanitizeComposerText keeps the composer free of terminal-breaking controls.
// Tabs become spaces; other C0 controls (except newline) are dropped. Raw tabs
// painted into the tty expand to tab-stops and desync the cell renderer.
// Block-element UI chrome (e.g. ▎ from transcript selection) is also stripped —
// those glyphs can disagree with tty ambiguous-width and shift the caret.
func sanitizeComposerText(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if !strings.ContainsFunc(s, func(r rune) bool {
		return r == '\t' || (r < 0x20 && r != '\n') || r == 0x7f || components.IsTranscriptChrome(string(r))
	}) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n':
			b.WriteByte('\n')
		case r == '\t':
			b.WriteString("    ")
		case r < 0x20, r == 0x7f:
			// drop
		case components.IsTranscriptChrome(string(r)):
			// drop transcript chrome
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (c *ChatInput) notifyChange() {
	if c.OnChange != nil {
		c.OnChange(c.Value)
	}
	c.notifyCompleters()
}

func (c *ChatInput) notifyCompleters() {
	c.notifyMention()
	c.notifySlash()
	c.notifySlashArg()
}

func (c *ChatInput) notifyMention() {
	if c.OnMentionChange == nil {
		return
	}
	q, _, _, ok := ActiveMention(c.Value, c.Cursor)
	c.OnMentionChange(ok, q)
}

func (c *ChatInput) notifySlash() {
	if c.OnSlashChange == nil {
		return
	}
	q, _, _, ok := ActiveSlash(c.Value, c.Cursor)
	c.OnSlashChange(ok, q)
}

func (c *ChatInput) notifySlashArg() {
	if c.OnSlashArgChange == nil {
		return
	}
	name, args, partial, _, _, ok := ActiveSlashArg(c.Value, c.Cursor)
	c.OnSlashArgChange(ok, name, args, partial)
}

// ReplaceRange replaces value[start:end] with text and places the cursor after it.
func (c *ChatInput) ReplaceRange(start, end int, text string) {
	if start < 0 {
		start = 0
	}
	if end > len(c.Value) {
		end = len(c.Value)
	}
	if start > end {
		start, end = end, start
	}
	text = sanitizeComposerText(text)
	c.Value = c.Value[:start] + text + c.Value[end:]
	c.Cursor = start + len(text)
	c.ClearSelection()
	c.notifyChange()
}

func (c *ChatInput) moveVert(delta int) {
	start := lineStart(c.Value, c.Cursor)
	col := utf8.RuneCountInString(c.Value[start:c.Cursor])
	if delta < 0 {
		if start == 0 {
			return // no line above; a single-line draft has nowhere to go
		}
		prevEnd := start - 1 // newline
		prevStart := lineStart(c.Value, prevEnd)
		c.Cursor = runeIndex(c.Value[prevStart:prevEnd], col) + prevStart
		return
	}
	end := lineEnd(c.Value, c.Cursor)
	if end >= len(c.Value) {
		return // no line below
	}
	nextStart := end + 1
	nextEnd := lineEnd(c.Value, nextStart)
	c.Cursor = runeIndex(c.Value[nextStart:nextEnd], col) + nextStart
}

// paintSelection tints the selected cells inside the editor region with the
// theme selection colors; rows fully inside the selection light up whole.
func (c *ChatInput) paintSelection(
	s *components.Surface,
	rows []visRow,
	scroll, editorRows, editorTop, textX, w int,
	th components.Theme,
) {
	selA, selB := c.selectionRange()
	if selB <= selA {
		return
	}
	for i := range editorRows {
		li := i + scroll
		if li < 0 || li >= len(rows) {
			continue
		}
		fromCol, toCol, ok := rowSelectionCols(&rows[li], selA, selB)
		if !ok {
			continue
		}
		tintCells(s, editorTop+i, textX+fromCol, textX+toCol, w, th)
	}
}

// tintCells overlays one run of editor cells with the theme selection
// colors — the one painter behind both the text selection and the
// reverse-i-search match highlight.
func tintCells(s *components.Surface, y, fromX, toX, w int, th components.Theme) {
	for x := fromX; x < toX && x < w; x++ {
		idx := y*w + x
		if idx < 0 || idx >= len(s.Buffer) {
			continue
		}
		cell := s.Buffer[idx]
		cell.Style.Bg = th.SelectionBg.Bg
		if th.SelectionFg.Fg.Kind != 0 {
			cell.Style.Fg = th.SelectionFg.Fg
		}
		s.Buffer[idx] = cell
	}
}

// Draw renders the framed composer: bar, panel, editor, meta row, tail, hints.
func (c *ChatInput) Draw(ctx components.DrawContext) components.Surface {
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	pendingH := c.pendingSkillsHeight()
	editorRows := c.bodyRows(w, ctx.Method)
	body := pendingH + editorRows // content rows inside the frame
	h := body + 5                 // pad + body + gap + meta + tail + hints
	if ctx.Max.Height > 0 && h > ctx.Max.Height {
		h = ctx.Max.Height
		body = h - 5
		if body < 1+pendingH {
			body = 1 + pendingH
			h = body + 5
		}
		editorRows = body - pendingH
		if editorRows < 1 {
			editorRows = 1
			body = pendingH + editorRows
			h = body + 5
		}
	}

	th := c.Theme
	if th.Foreground.Fg.Kind == 0 && th.Muted.Fg.Kind == 0 {
		th = components.DefaultTheme()
	}
	textSt := c.TextStyle
	if textSt == (xui.Style{}) {
		textSt = th.Foreground
	}
	cursorSt := c.CursorStyle
	if cursorSt == (xui.Style{}) {
		cursorSt = xui.Style{Reverse: true}
	}
	barSt := c.AgentLabel.Style
	if barSt == (xui.Style{}) {
		barSt = xui.Style{Fg: xui.IndexedColor(240)}
	}
	panelBg := th.BackgroundElement.Bg
	hasPanel := !panelBg.Equal(xui.DefaultColor())
	// Print replaces cell styles wholesale, so every style painted inside
	// the panel must carry the panel bg — a bgless span would punch a
	// default-background hole into the element panel. The hints row sits
	// below the frame and keeps the terminal background (raw th).
	panelTh := th
	metaLead := barSt
	if hasPanel {
		textSt.Bg = panelBg
		panelTh.Muted.Bg = panelBg
		panelTh.Foreground.Bg = panelBg
		metaLead.Bg = panelBg
	}

	s := components.NewSurface(w, h, c)
	metaY := body + 2
	tailY := body + 3
	hintsY := body + 4

	// Left posture bar spans the frame; the ╹ tail closes it below.
	for y := 0; y <= metaY; y++ {
		s.SetCell(0, y, xui.Cell{Char: "┃", Width: 1, Style: barSt})
	}
	s.SetCell(0, tailY, xui.Cell{Char: "╹", Width: 1, Style: barSt})

	// Panel fill behind the frame; non-default bg so Clear+diff works cleanly.
	for y := 0; y <= metaY; y++ {
		st := textSt
		if hasPanel {
			st = xui.Style{Bg: panelBg}
		}
		for x := 1; x < w; x++ {
			s.SetCell(x, y, xui.Cell{Char: " ", Width: 1, Style: st})
		}
	}

	// Tail fade: ▀ in the panel color merges the frame into the terminal bg.
	if hasPanel {
		for x := 1; x < w; x++ {
			s.SetCell(x, tailY, xui.Cell{Char: "▀", Width: 1, Style: xui.Style{Fg: panelBg}})
		}
	}

	textX := 3 // bar + paddingLeft 2
	innerW := w - 5
	innerW = max(innerW, 1)

	if pendingH > 0 {
		c.paintPendingSkills(&s, textX, 1, innerW, panelTh, ctx.Method)
	}

	// The body shows the draft, except in reverse-i-search: there it previews
	// the current match (query highlighted) or a muted no-matches line; the
	// draft itself waits in Value until a key accepts the match.
	bodyText, bodySt := c.Value, textSt
	if m, ok := c.searchMatch(); ok {
		bodyText = m
	} else if c.search.active && c.search.query != "" {
		bodyText, bodySt = "no matches", panelTh.Muted
	}
	caret := c.Cursor
	if c.search.active {
		caret = len(bodyText) // a read-only preview parks the caret at its end
	}
	rows := layoutEditor(bodyText, innerW, ctx.Method)
	c.rows, c.rowsW, c.rowsScroll = rows, innerW, 0
	// Scroll so the cursor's visual row stays visible within the editor region.
	curLine, curCol := offsetToRowCol(rows, caret, innerW)
	scroll := 0
	if curLine >= editorRows {
		scroll = curLine - editorRows + 1
	}
	c.rowsScroll = scroll
	editorTop := 1 + pendingH
	for i := range editorRows {
		li := i + scroll
		if li < 0 || li >= len(rows) {
			continue
		}
		s.Print(textX, editorTop+i, rows[li].text, bodySt, ctx.Method)
	}
	if c.hasSel {
		c.paintSelection(&s, rows, scroll, editorRows, editorTop, textX, w, th)
	}
	if m, ok := c.searchMatch(); ok && c.search.query != "" {
		c.paintSearchHit(&s, rows, m, scroll, editorRows, editorTop, textX, w, th)
	}

	if bodyText == "" && !c.search.active && c.Placeholder != "" {
		// The placeholder is a draft affordance: the search preview (match or
		// "no matches") owns the body while the mode is on, and Value stays the
		// untouched draft — often empty — so judging by Value would paint the
		// placeholder right over the previewed match.
		s.Print(textX, editorTop, layout.TruncateToWidth(c.Placeholder, innerW, ctx.Method), panelTh.Muted, ctx.Method)
	}

	c.paintMetaRow(&s, textX, metaY, panelTh, metaLead, ctx.Method)
	c.paintHintsRow(&s, hintsY, w, th, ctx.Method)

	// Cursor position in surface coords (editor region, below skills).
	visLine := curLine - scroll
	visLine = max(visLine, 0)
	if visLine >= editorRows {
		visLine = editorRows - 1
	}
	cx := textX + curCol
	cy := editorTop + visLine
	if cx >= w-1 {
		cx = w - 2
	}
	if cy >= h-1 {
		cy = h - 2
	}
	// Never place the block cursor on a wide-glyph continuation column —
	// that paints a Width=1 reverse space into the second half of a CJK cell
	// and shows up as phantom "cursor blocks".
	cx = text.SnapSurfaceColToGlyphStart(s.Buffer, w, cx, cy)

	if c.UseBlockCursor {
		existing := s.Buffer[cy*w+cx]
		ch := existing.Char
		width := existing.Width
		if ch == "" {
			ch = " "
		}
		if width < 1 {
			width = 1
		}
		// If this cell is somehow still a trail, don't reverse-paint it.
		if width == 1 && cx > 0 {
			prev := s.Buffer[cy*w+cx-1]
			if prev.Width > 1 {
				cx--
				existing = s.Buffer[cy*w+cx]
				ch = existing.Char
				width = existing.Width
				if ch == "" {
					ch = " "
				}
				if width < 1 {
					width = 1
				}
			}
		}
		s.SetCell(cx, cy, xui.Cell{Char: ch, Width: width, Style: cursorSt})
	}
	s.Cursor = &components.Point{X: cx, Y: cy}

	if debuglog.Enabled() && c.dumpNextDraw {
		c.dumpNextDraw = false
		debuglog.Logf(
			"chat draw w=%d innerW=%d body=%d editorRows=%d pending=%d lines=%d curLine=%d curCol=%d scroll=%d cx=%d cy=%d",
			w,
			innerW,
			body,
			editorRows,
			pendingH,
			len(rows),
			curLine,
			curCol,
			scroll,
			cx,
			cy,
		)
		if visLine >= 0 && curLine-scroll < len(rows) && curLine-scroll >= 0 {
			li := curLine - scroll
			debuglog.Logf("chat draw focus line %q width=%d", rows[li].text, xui.StringWidth(rows[li].text, ctx.Method))
		}
		cell := s.Buffer[cy*w+cx]
		debuglog.Logf("chat cursor cell char=%q width=%d reverse=%v", cell.Char, cell.Width, cell.Style.Reverse)
		dumpSurfaceRow("chat row", s.Buffer, w, cy)
	}
	return s
}

// paintMetaRow paints the in-frame posture/model row: "⏵⏵ build · model",
// or the reverse-i-search prompt in the posture's place while the mode is on.
func (c *ChatInput) paintMetaRow(
	s *components.Surface,
	x, y int,
	th components.Theme,
	lead xui.Style,
	method xui.WidthMethod,
) {
	var spans []components.Span
	if c.search.active {
		spans = c.searchMetaSpans(lead, th)
	} else {
		if c.AgentLabel.Text == "" && c.ModelLabel == "" {
			return
		}
		if c.AgentLabel.Text != "" {
			spans = append(spans, components.Span{Text: c.AgentLabel.Text, Style: lead})
		}
		if c.ModelLabel != "" {
			if len(spans) > 0 {
				spans = append(spans, components.Span{Text: " · ", Style: th.Muted})
			}
			spans = append(spans, components.Span{Text: c.ModelLabel, Style: th.Foreground})
		}
	}
	components.PaintSpans(s, x, y, spans, method)
}

// paintHintsRow paints the row below the frame: cwd muted on the left, usage
// spans right-aligned (keymap fallback when empty).
func (c *ChatInput) paintHintsRow(s *components.Surface, y, w int, th components.Theme, method xui.WidthMethod) {
	if c.HintsLeft != "" {
		s.Print(1, y, c.HintsLeft, th.Muted, method)
	}
	right := c.HintsRight
	if len(right) == 0 {
		right = []components.Span{
			{Text: "tab", Style: th.Foreground},
			{Text: " mode", Style: th.Muted},
			{Text: "  ", Style: th.Muted},
			{Text: "^k", Style: th.Foreground},
			{Text: " commands", Style: th.Muted},
			{Text: "  ", Style: th.Muted},
			{Text: "^r", Style: th.Foreground},
			{Text: " history", Style: th.Muted},
		}
	}
	total := 0
	for _, sp := range right {
		total += xui.StringWidth(sp.Text, method)
	}
	x := w - total - 1
	x = max(x, 1)
	components.PaintSpans(s, x, y, right, method)
}

func (c *ChatInput) paintPendingSkills(
	s *components.Surface,
	x, y, width int,
	th components.Theme,
	method xui.WidthMethod,
) {
	labelSt := th.Muted
	labelSt.Dim = true
	nameSt := th.Success
	nameSt.Bold = false
	nameSt.Underline = true

	spans := []components.Span{{Text: "Skills: ", Style: labelSt}}
	for i, name := range c.PendingSkills {
		if i > 0 {
			spans = append(spans, components.Span{Text: " ", Style: labelSt})
		}
		spans = append(spans, components.Span{Text: name, Style: nameSt})
	}
	lines := components.WrapSpans(spans, width, method)
	if len(lines) == 0 {
		return
	}
	components.PaintSpans(s, x, y, lines[0], method)
}

func lineStart(s string, off int) int {
	if off > len(s) {
		off = len(s)
	}
	i := strings.LastIndexByte(s[:off], '\n')
	if i < 0 {
		return 0
	}
	return i + 1
}

func lineEnd(s string, off int) int {
	if off > len(s) {
		off = len(s)
	}
	i := strings.IndexByte(s[off:], '\n')
	if i < 0 {
		return len(s)
	}
	return off + i
}

func runeIndex(s string, n int) int {
	if n <= 0 {
		return 0
	}
	i := 0
	for pos := range s {
		if i == n {
			return pos
		}
		i++
	}
	return len(s)
}

func dumpSurfaceRow(label string, buf []xui.Cell, rowW, row int) {
	if buf == nil || row < 0 || rowW < 1 {
		return
	}
	var b strings.Builder
	b.WriteString(label)
	b.WriteByte(':')
	for x := 0; x < rowW; {
		i := row*rowW + x
		if i >= len(buf) {
			break
		}
		c := buf[i]
		step := int(c.Width)
		step = max(step, 1)
		ch := c.Char
		if ch == "" || ch == " " {
			ch = "·"
		}
		b.WriteByte(' ')
		b.WriteString(ch)
		if c.Style.Reverse {
			b.WriteString("!R")
		}
		if step > 1 {
			b.WriteByte('x')
			b.WriteByte(byte('0' + min(step, 9))) //nolint:gosec // G115: step clamped to 0..9
		}
		x += step
	}
	debuglog.Logf("%s", b.String())
}
