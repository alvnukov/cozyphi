package input

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
)

func typeInto(l *Line, s string) {
	for _, r := range s {
		l.Key(xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: r})
	}
}

func TestLineTypeAndWordDelete(t *testing.T) {
	var l Line
	typeInto(&l, "one two")
	if l.Value != "one two" || l.Cursor != len("one two") {
		t.Fatalf("typed %q cursor=%d", l.Value, l.Cursor)
	}
	// A word delete eats the whitespace gap too, like the composer.
	l.Key(xui.KeyEvent{Press: true, Code: xui.KeyBackspace, Mods: xui.ModCtrl})
	if l.Value != "one" || l.Cursor != 3 {
		t.Fatalf("word delete %q cursor=%d", l.Value, l.Cursor)
	}
	l.Key(xui.KeyEvent{Press: true, Code: xui.KeyBackspace})
	if l.Value != "on" {
		t.Fatalf("backspace %q", l.Value)
	}
}

func TestLineCaretMotion(t *testing.T) {
	var l Line
	typeInto(&l, "héllo")
	l.Key(xui.KeyEvent{Press: true, Code: xui.KeyHome})
	if l.Cursor != 0 {
		t.Fatalf("home cursor=%d", l.Cursor)
	}
	l.Key(xui.KeyEvent{Press: true, Code: xui.KeyRight})
	l.Key(xui.KeyEvent{Press: true, Code: xui.KeyDelete})
	if l.Value != "hllo" {
		t.Fatalf("delete over multi-byte rune %q", l.Value)
	}
	// Left walks whole runes back over the multi-byte one it just skipped.
	typeInto(&l, "é")
	l.Key(xui.KeyEvent{Press: true, Code: xui.KeyLeft})
	if l.Cursor != 1 {
		t.Fatalf("left cursor=%d", l.Cursor)
	}
	l.Key(xui.KeyEvent{Press: true, Code: xui.KeyEnd})
	if l.Cursor != len(l.Value) {
		t.Fatalf("end cursor=%d value=%q", l.Cursor, l.Value)
	}
}

func TestLineLeavesForeignKeys(t *testing.T) {
	var l Line
	for _, e := range []xui.KeyEvent{
		{Press: true, Code: xui.KeyEnter},
		{Press: true, Code: xui.KeyEscape},
		{Press: true, Code: xui.KeyTab},
		{Press: true, Code: xui.KeyUp},
		{Press: true, Code: xui.KeyDown},
		{Press: true, Code: xui.KeyRune, Rune: 'a', Mods: xui.ModCtrl},
		{Press: false, Code: xui.KeyRune, Rune: 'a'},
	} {
		if l.Key(e) {
			t.Fatalf("claimed %+v", e)
		}
	}
	if l.Value != "" {
		t.Fatalf("value %q", l.Value)
	}
}

func TestLineInsertSanitizes(t *testing.T) {
	var l Line
	if !l.Insert("a\r\nb\tc\x00" + CaretGlyph) {
		t.Fatal("insert reported nothing landed")
	}
	if l.Value != "a b c" {
		t.Fatalf("sanitized %q", l.Value)
	}
}

func TestLineMaxRunes(t *testing.T) {
	l := Line{MaxRunes: 4}
	if !l.Insert("héllo") {
		t.Fatal("insert reported nothing landed")
	}
	if l.Value != "héll" {
		t.Fatalf("capped %q", l.Value)
	}
	if l.Insert("more") {
		t.Fatal("insert past the cap reported success")
	}
}

func TestLineDisplayScrolls(t *testing.T) {
	const width = 20
	l := Line{}
	l.Set(strings.Repeat("ab", 40))

	out := l.Display(width, xui.WidthUnicode)
	if xui.StringWidth(out, xui.WidthUnicode) > width {
		t.Fatalf("caret at end overflows: %q", out)
	}
	if !strings.HasSuffix(out, CaretGlyph) {
		t.Fatalf("caret at end not shown: %q", out)
	}

	l.Key(xui.KeyEvent{Press: true, Code: xui.KeyHome})
	out = l.Display(width, xui.WidthUnicode)
	if w := xui.StringWidth(out, xui.WidthUnicode); w > width || w < width-1 {
		t.Fatalf("caret at start wastes the row: %q width=%d", out, w)
	}
	if !strings.HasPrefix(out, CaretGlyph) {
		t.Fatalf("caret at start not shown: %q", out)
	}

	// A value that fits keeps every column of it, caret included.
	l.Set("short")
	if out := l.Display(width, xui.WidthUnicode); out != "short"+CaretGlyph {
		t.Fatalf("short value %q", out)
	}
}
