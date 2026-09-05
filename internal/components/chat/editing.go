package chat

import (
	"slices"
	"strings"
	"unicode"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/text"
	"github.com/alvnukov/cozyphi/internal/editmode"
)

type editSnapshot struct {
	value  string
	cursor int
}

type editingState struct {
	mode            editmode.Mode
	normal          bool
	pending         rune
	register        string
	linewise        bool
	undo            []editSnapshot
	redo            []editSnapshot
	observed        string
	restoring       bool
	tracking        bool
	vimInsertStart  editSnapshot
	vimInsertActive bool
}

// SetEditingMode changes the keyboard dialect without replacing the draft.
func (c *ChatInput) SetEditingMode(mode editmode.Mode) {
	if c.edit.observed != c.Value {
		c.invalidateEdits()
	} else {
		c.finishVimInsert()
	}
	c.edit.observed = c.Value
	c.edit.mode = mode
	c.edit.normal = false
	c.edit.pending = 0
	c.notifyCompleters()
}

func (c *ChatInput) EditingMode() editmode.Mode { return c.edit.mode }

// EditingLabel stays visible even when usage statistics replace the hints.
func (c *ChatInput) EditingLabel() string {
	if c.edit.mode == editmode.Vim {
		if c.edit.normal {
			if c.edit.pending != 0 {
				return "VIM NORMAL · " + string(c.edit.pending)
			}
			return "VIM NORMAL"
		}
		return "VIM INSERT"
	}
	return strings.ToUpper(c.edit.mode.String())
}

func (c *ChatInput) snapshotEdit() editSnapshot {
	return editSnapshot{value: c.Value, cursor: c.Cursor}
}

// trackEdit observes public input events, not the low-level insert/delete pair:
// replacing a selection is one operation, and a Vim INSERT session is one
// operation from the command that entered it through the closing Escape.
func (c *ChatInput) trackEdit() func() {
	if c.edit.tracking {
		return func() {}
	}
	c.edit.tracking = true
	before := c.snapshotEdit()
	beforeNormal := c.edit.normal
	if c.edit.observed != c.Value {
		c.invalidateEdits()
	}
	c.edit.restoring = false
	return func() {
		c.edit.tracking = false
		if c.edit.restoring {
			c.edit.observed = c.Value
			return
		}

		changed := before.value != c.Value
		if c.edit.mode != editmode.Vim {
			if changed {
				c.pushUndo(before)
			}
			c.edit.observed = c.Value
			return
		}

		switch {
		case beforeNormal && !c.edit.normal:
			c.beginVimInsert(before)
			if changed {
				c.edit.redo = nil
			}
		case !beforeNormal:
			if changed && !c.edit.vimInsertActive {
				c.beginVimInsert(before)
			}
			if changed {
				c.edit.redo = nil
			}
			if c.edit.normal {
				c.finishVimInsert()
			}
		case changed:
			c.pushUndo(before)
		}
		c.edit.observed = c.Value
	}
}

func (c *ChatInput) invalidateEdits() {
	c.edit.undo, c.edit.redo = nil, nil
	c.edit.vimInsertStart = editSnapshot{}
	c.edit.vimInsertActive = false
}

func (c *ChatInput) pushUndo(before editSnapshot) {
	c.edit.undo = boundedEdits(c.edit.undo, before)
	c.edit.redo = nil
}

func (c *ChatInput) beginVimInsert(before editSnapshot) {
	if c.edit.vimInsertActive {
		return
	}
	c.edit.vimInsertStart = before
	c.edit.vimInsertActive = true
}

func (c *ChatInput) finishVimInsert() {
	if !c.edit.vimInsertActive {
		return
	}
	before := c.edit.vimInsertStart
	c.edit.vimInsertStart = editSnapshot{}
	c.edit.vimInsertActive = false
	if before.value != c.Value {
		c.pushUndo(before)
	}
}

// Keep at most 100 revisions and 1 MiB of draft text in each direction.
func boundedEdits(stack []editSnapshot, entry editSnapshot) []editSnapshot {
	const budget = 1 << 20
	if len(entry.value) > budget {
		return nil
	}
	stack = append(stack, entry)
	total := 0
	for i := range slices.Backward(stack) {
		total += len(stack[i].value)
		if total > budget || len(stack)-i > 100 {
			return append([]editSnapshot(nil), stack[i+1:]...)
		}
	}
	return stack
}

func (c *ChatInput) undoEdit(redo bool) {
	c.finishVimInsert()
	from, to := &c.edit.undo, &c.edit.redo
	if redo {
		from, to = to, from
	}
	c.edit.restoring = true
	if len(*from) == 0 {
		return
	}
	*to = boundedEdits(*to, c.snapshotEdit())
	last := len(*from) - 1
	state := (*from)[last]
	(*from)[last] = editSnapshot{}
	*from = (*from)[:last]
	c.Value, c.Cursor = state.value, state.cursor
	c.ClearSelection()
	c.notifyChange()
}

func (c *ChatInput) handleEditingKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	if c.search.active {
		return false
	}
	if c.edit.mode == editmode.Vim && c.handleVimKey(ctx, e) {
		return true
	}
	if e.Code == xui.KeyRune && (e.Mods == xui.ModCtrl || e.Mods == xui.ModCtrl|xui.ModShift) {
		switch e.HotkeyRune() {
		case 'z', 'Z':
			c.undoEdit(e.Mods.Has(xui.ModShift))
			ctx.ConsumeAndRedraw()
			return true
		case 'y':
			if c.edit.mode != editmode.Readline {
				c.undoEdit(true)
				ctx.ConsumeAndRedraw()
				return true
			}
		}
	}
	if c.edit.mode == editmode.Readline && c.handleReadline(e) {
		ctx.ConsumeAndRedraw()
		return true
	}
	return false
}

func (c *ChatInput) handleReadline(e xui.KeyEvent) bool {
	if e.Code == xui.KeyBackspace && e.Mods == xui.ModAlt {
		c.killEdit(text.PrevWordStart(c.Value, c.Cursor), c.Cursor)
		return true
	}
	if e.Code != xui.KeyRune {
		return false
	}
	key := unicode.ToLower(e.HotkeyRune())
	if e.Mods == xui.ModAlt {
		switch key {
		case 'b':
			c.moveTo(text.PrevWordStart(c.Value, c.Cursor), false)
		case 'f':
			c.moveTo(text.NextWordEnd(c.Value, c.Cursor), false)
		case 'd':
			c.killEdit(c.Cursor, text.NextWordEnd(c.Value, c.Cursor))
		default:
			return false
		}
		c.notifyCompleters()
		return true
	}
	if e.Mods != xui.ModCtrl {
		return false
	}
	switch key {
	case 'a':
		c.moveTo(lineStart(c.Value, c.Cursor), false)
	case 'e':
		c.moveTo(lineEnd(c.Value, c.Cursor), false)
	case 'b':
		c.arrowLeft(xui.KeyEvent{})
	case 'f':
		c.arrowRight(xui.KeyEvent{})
	case 'h':
		c.backspace(false)
	case 'd':
		c.deleteForward(false)
	case 'p':
		if !c.recall(xui.KeyUp) {
			c.moveVert(-1)
		}
	case 'n':
		if !c.recall(xui.KeyDown) {
			c.moveVert(1)
		}
	case 'u':
		c.killEdit(lineStart(c.Value, c.Cursor), c.Cursor)
	case 'k':
		end := lineEnd(c.Value, c.Cursor)
		if end == c.Cursor && end < len(c.Value) {
			end++
		}
		c.killEdit(c.Cursor, end)
	case 'w':
		start := text.SkipLeftWhile(c.Value, c.Cursor, unicode.IsSpace)
		start = text.SkipLeftWhile(c.Value, start, func(r rune) bool { return !unicode.IsSpace(r) })
		c.killEdit(start, c.Cursor)
	case 'y':
		c.insert(c.edit.register)
	default:
		return false
	}
	c.notifyCompleters()
	return true
}

func (c *ChatInput) killEdit(start, end int) {
	if c.HasSelection() {
		start, end = c.selectionRange()
	}
	if start >= end {
		return
	}
	c.edit.register = c.Value[start:end]
	c.edit.linewise = false
	c.deleteRange(start, end)
}
