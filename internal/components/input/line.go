package input

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/components/text"
)

// CaretGlyph marks the insertion point in a rendered Line.
const CaretGlyph = "▎"

// Line is the one-row text model the modal prompts share: the permission ask's
// deny-with-feedback field, the question overlay's custom answer, the connect
// overlay's provider filter. Each of those hand-rolled its own key handling and
// they disagreed — one had a caret, the others only appended, none of them took
// a paste — so which editing keys worked depended on which modal happened to be
// up. One model settles that for all of them.
//
// It is a model, not a widget: it owns the value, the caret and the key set,
// while the caller keeps drawing the row into the panel it already owns.
type Line struct {
	Value    string
	Cursor   int // byte offset into Value, like TextField and ChatInput
	MaxRunes int // hard cap on the value; 0 means unlimited
}

// Set replaces the value and parks the caret at its end.
func (l *Line) Set(s string) {
	l.Value = sanitizeLine(s)
	l.Cursor = len(l.Value)
}

// Clear empties the line.
func (l *Line) Clear() {
	l.Value = ""
	l.Cursor = 0
}

// Trimmed returns the value without surrounding space — what a prompt submits.
func (l *Line) Trimmed() string { return strings.TrimSpace(l.Value) }

// Empty reports whether the line would submit nothing.
func (l *Line) Empty() bool { return l.Trimmed() == "" }

// Key applies one key press, reporting whether the key belongs to line editing
// (it may still have had nothing to do — Backspace on an empty line is taken,
// not passed on). A key the line does not claim comes back false so the modal
// above can answer it: Enter, Esc, Tab, the arrows that walk a list, and every
// Ctrl/Alt chord that is not a word motion.
func (l *Line) Key(e xui.KeyEvent) bool {
	if !e.Press {
		return false
	}
	l.clamp()
	word := e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt)
	switch e.Code {
	case xui.KeyRune:
		if word {
			return false
		}
		l.Insert(string(e.Rune))
	case xui.KeyBackspace:
		l.deleteLeft(word)
	case xui.KeyDelete:
		l.deleteRight(word)
	case xui.KeyLeft:
		if word {
			l.Cursor = text.PrevWordStart(l.Value, l.Cursor)
		} else if l.Cursor > 0 {
			_, size := utf8.DecodeLastRuneInString(l.Value[:l.Cursor])
			l.Cursor -= size
		}
	case xui.KeyRight:
		if word {
			l.Cursor = text.NextWordEnd(l.Value, l.Cursor)
		} else if l.Cursor < len(l.Value) {
			_, size := utf8.DecodeRuneInString(l.Value[l.Cursor:])
			l.Cursor += size
		}
	case xui.KeyHome:
		l.Cursor = 0
	case xui.KeyEnd:
		l.Cursor = len(l.Value)
	default:
		return false
	}
	return true
}

// Insert adds s at the caret and reports whether anything landed. A paste the
// cap rejects whole says so, so the caller can show why nothing appeared.
func (l *Line) Insert(s string) bool {
	s = sanitizeLine(s)
	if s == "" {
		return false
	}
	l.clamp()
	if l.MaxRunes > 0 {
		room := l.MaxRunes - utf8.RuneCountInString(l.Value)
		if room <= 0 {
			return false
		}
		s = truncateRunes(s, room)
		if s == "" {
			return false
		}
	}
	l.Value = l.Value[:l.Cursor] + s + l.Value[l.Cursor:]
	l.Cursor += len(s)
	return true
}

// Display renders the value with the caret at the cursor, scrolled so the caret
// stays inside width columns. These prompts draw into one row of someone else's
// panel: without scrolling, a value longer than the row ran off the end and took
// the caret with it, and typing looked like it had stopped working.
func (l *Line) Display(width int, method xui.WidthMethod) string {
	l.clamp()
	if width <= 1 {
		return CaretGlyph
	}
	head, tail := l.Value[:l.Cursor], l.Value[l.Cursor:]
	if xui.StringWidth(l.Value, method)+1 <= width {
		return head + CaretGlyph + tail
	}
	// Keep a slice of what follows the caret in view, so editing the middle of a
	// long value still shows what is being edited.
	tailW := xui.StringWidth(tail, method)
	keep := min(max((width-1)/3, 1), tailW)
	head = tailToWidth(head, width-1-keep, method)
	// Whatever the head left unused goes back to the tail, so a caret parked at
	// the start of a long value still fills the row with what follows it.
	keep = min(width-1-xui.StringWidth(head, method), tailW)
	return head + CaretGlyph + layout.TruncateToWidth(tail, keep, method)
}

func (l *Line) clamp() {
	if l.Cursor < 0 {
		l.Cursor = 0
	}
	if l.Cursor > len(l.Value) {
		l.Cursor = len(l.Value)
	}
}

func (l *Line) deleteLeft(word bool) {
	from := l.Cursor
	switch {
	case word:
		from = text.PrevWordStart(l.Value, l.Cursor)
		// Consume the whitespace gap before the word too, exactly as the composer
		// does: deleting "two" out of "one two" yields "one", not "one ".
		from = text.SkipLeftWhile(l.Value, from, unicode.IsSpace)
	case from > 0:
		_, size := utf8.DecodeLastRuneInString(l.Value[:from])
		from -= size
	}
	if from == l.Cursor {
		return
	}
	l.Value = l.Value[:from] + l.Value[l.Cursor:]
	l.Cursor = from
}

func (l *Line) deleteRight(word bool) {
	to := l.Cursor
	switch {
	case word:
		to = text.NextWordEnd(l.Value, l.Cursor)
	case to < len(l.Value):
		_, size := utf8.DecodeRuneInString(l.Value[to:])
		to += size
	}
	if to == l.Cursor {
		return
	}
	l.Value = l.Value[:l.Cursor] + l.Value[to:]
}

// sanitizeLine flattens s to a single row: a newline or tab becomes a space
// (there is no second row to put it on), control runes are dropped, and so is
// transcript chrome — CaretGlyph pasted into the value would read as a caret.
func sanitizeLine(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n', r == '\r', r == '\t':
			b.WriteByte(' ')
		case r < 0x20, r == 0x7f:
			// drop
		case components.IsTranscriptChrome(string(r)):
			// drop
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// truncateRunes returns the first n runes of s.
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}

// tailToWidth returns the longest suffix of s that fits within width columns.
func tailToWidth(s string, width int, method xui.WidthMethod) string {
	if width <= 0 {
		return ""
	}
	total := xui.StringWidth(s, method)
	rest := s
	for rest != "" && total > width {
		_, cw, next := xui.FirstGrapheme(rest, method)
		total -= max(cw, 1)
		rest = next
	}
	return rest
}
