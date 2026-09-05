package chat

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/text"
)

// Vim is deliberately a composer dialect: Enter sends in INSERT, while NORMAL
// owns printable keys and Enter never submits a draft by accident.
func (c *ChatInput) handleVimKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	if c.VoiceMode || c.completerOpen() {
		return false
	}
	if e.Code == xui.KeyEscape && e.Mods == 0 {
		if !c.edit.normal {
			c.edit.normal = true
			if c.Cursor > lineStart(c.Value, c.Cursor) {
				c.Cursor = prevGrapheme(c.Value, c.Cursor)
			}
			c.ClearSelection()
			c.notifyCompleters()
		}
		c.edit.pending = 0
		ctx.ConsumeAndRedraw()
		return true
	}
	if !c.edit.normal {
		return false
	}
	if e.Code == xui.KeyRune && e.Mods == xui.ModCtrl && e.HotkeyRune() == 'r' {
		c.edit.pending = 0
		c.undoEdit(true)
		c.clampNormal()
		ctx.ConsumeAndRedraw()
		return true
	}
	if e.Code == xui.KeyBackspace && e.Mods == 0 {
		c.edit.pending = 0
		c.vimCommand('h')
		ctx.ConsumeAndRedraw()
		return true
	}
	if e.Code == xui.KeyEnter && e.Mods == 0 {
		ctx.ConsumeAndRedraw()
		return true
	}
	if e.Code != xui.KeyRune || (e.Mods != 0 && e.Mods != xui.ModShift) {
		return false
	}
	key := e.HotkeyRune()
	// Kitty may report an unshifted alternate key even for an uppercase command.
	if e.Mods.Has(xui.ModShift) || unicode.IsUpper(e.Rune) {
		key = unicode.ToUpper(key)
	}
	if c.edit.pending != 0 {
		op := c.edit.pending
		c.edit.pending = 0
		c.vimOperator(op, key)
	} else {
		c.vimCommand(key)
	}
	c.clampNormal()
	c.notifyCompleters()
	ctx.ConsumeAndRedraw()
	return true
}

func (c *ChatInput) clampNormal() {
	if !c.edit.normal {
		return
	}
	end := lineEnd(c.Value, c.Cursor)
	if c.Cursor == end && end > lineStart(c.Value, c.Cursor) {
		c.Cursor = prevGrapheme(c.Value, c.Cursor)
	}
}

func (c *ChatInput) vimCommand(key rune) {
	switch key {
	case 'h':
		c.moveTo(max(lineStart(c.Value, c.Cursor), prevGrapheme(c.Value, c.Cursor)), false)
	case 'l':
		c.moveTo(min(lineEnd(c.Value, c.Cursor), nextGrapheme(c.Value, c.Cursor)), false)
	case 'j':
		c.moveVert(1)
	case 'k':
		c.moveVert(-1)
	case 'w':
		c.moveTo(vimWordNext(c.Value, c.Cursor), false)
	case 'b':
		c.moveTo(vimWordPrevious(c.Value, c.Cursor), false)
	case '0':
		c.moveTo(lineStart(c.Value, c.Cursor), false)
	case '^':
		start := lineStart(c.Value, c.Cursor)
		end := lineEnd(c.Value, c.Cursor)
		for start < end && (c.Value[start] == ' ' || c.Value[start] == '\t') {
			start++
		}
		c.moveTo(start, false)
	case '$':
		c.moveTo(lineEnd(c.Value, c.Cursor), false)
	case 'G':
		c.moveTo(lineStart(c.Value, len(c.Value)), false)
	case 'g', 'd', 'c', 'y':
		c.edit.pending = key
	case 'i':
		c.edit.normal = false
	case 'a':
		c.Cursor = min(lineEnd(c.Value, c.Cursor), nextGrapheme(c.Value, c.Cursor))
		c.edit.normal = false
	case 'I':
		c.vimCommand('^')
		c.edit.normal = false
	case 'A':
		c.Cursor = lineEnd(c.Value, c.Cursor)
		c.edit.normal = false
	case 'o':
		c.Cursor = lineEnd(c.Value, c.Cursor)
		c.edit.normal = false
		c.insert("\n")
	case 'O':
		c.Cursor = lineStart(c.Value, c.Cursor)
		c.edit.normal = false
		c.insert("\n")
		c.Cursor--
	case 'x':
		end := min(nextGrapheme(c.Value, c.Cursor), lineEnd(c.Value, c.Cursor))
		c.killEdit(c.Cursor, end)
	case 'D', 'C':
		c.killEdit(c.Cursor, lineEnd(c.Value, c.Cursor))
		if key == 'C' {
			c.edit.normal = false
		}
	case 'p':
		c.vimPut()
	case 'u':
		c.undoEdit(false)
	}
}

func (c *ChatInput) vimOperator(op, key rune) {
	if op == 'g' {
		if key == 'g' {
			c.moveTo(0, false)
		}
		return
	}
	if key == op {
		c.vimLineOperator(op)
		return
	}
	start := c.Cursor
	var end int
	switch key {
	case 'w':
		end = vimWordNext(c.Value, c.Cursor)
		if op == 'c' {
			r, _ := utf8.DecodeRuneInString(c.Value[c.Cursor:])
			if !unicode.IsSpace(r) {
				end = text.SkipLeftWhile(c.Value, end, unicode.IsSpace)
			}
		}
	case '$':
		end = lineEnd(c.Value, c.Cursor)
	default:
		return
	}
	if start < end {
		c.edit.register, c.edit.linewise = c.Value[start:end], false
		if op != 'y' {
			c.deleteRange(start, end)
		}
	}
	if op == 'c' {
		c.edit.normal = false
	}
}

func (c *ChatInput) vimLineOperator(op rune) {
	start, end := lineStart(c.Value, c.Cursor), lineEnd(c.Value, c.Cursor)
	c.edit.register, c.edit.linewise = c.Value[start:end]+"\n", true
	if op == 'y' {
		return
	}
	if op == 'c' {
		c.deleteRange(start, end)
		c.edit.normal = false
		return
	}
	if end < len(c.Value) {
		end++
	} else if start > 0 {
		start--
	}
	c.deleteRange(start, end)
	c.Cursor = lineStart(c.Value, c.Cursor)
}

func (c *ChatInput) vimPut() {
	if c.edit.register == "" {
		return
	}
	if c.edit.linewise {
		value := strings.TrimSuffix(c.edit.register, "\n")
		if c.Value == "" {
			c.insert(value)
			c.Cursor = 0
			return
		}
		end := lineEnd(c.Value, c.Cursor)
		c.Cursor = end
		c.insert("\n" + value)
		c.Cursor = end + 1
	} else {
		c.Cursor = min(lineEnd(c.Value, c.Cursor), nextGrapheme(c.Value, c.Cursor))
		c.insert(c.edit.register)
		c.Cursor = prevGrapheme(c.Value, c.Cursor)
	}
}

func vimWordNext(s string, off int) int {
	if off >= len(s) {
		return len(s)
	}
	r, _ := utf8.DecodeRuneInString(s[off:])
	class := wordClass(r)
	for off < len(s) {
		r, _ = utf8.DecodeRuneInString(s[off:])
		if wordClass(r) != class {
			break
		}
		off = nextGrapheme(s, off)
	}
	for off < len(s) {
		r, _ = utf8.DecodeRuneInString(s[off:])
		if !unicode.IsSpace(r) {
			break
		}
		off = nextGrapheme(s, off)
	}
	return off
}

func wordClass(r rune) int {
	if unicode.IsSpace(r) {
		return 0
	}
	if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' {
		return 1
	}
	return 2
}

func vimWordPrevious(s string, off int) int {
	off = prevGrapheme(s, off)
	for off > 0 {
		r, _ := utf8.DecodeRuneInString(s[off:])
		if !unicode.IsSpace(r) {
			break
		}
		off = prevGrapheme(s, off)
	}
	r, _ := utf8.DecodeRuneInString(s[off:])
	class := wordClass(r)
	for off > 0 {
		prev := prevGrapheme(s, off)
		r, _ = utf8.DecodeRuneInString(s[prev:])
		if wordClass(r) != class {
			break
		}
		off = prev
	}
	return off
}
