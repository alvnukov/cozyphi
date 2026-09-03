package keys

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pulseaiclub/xui"
)

// Chord is one parsed key combination: modifiers plus a named key or a
// printable rune. It is the matching side of the catalog — the spelling a
// user writes in config ("Ctrl+P", "Alt+P", "F1", "Ctrl+Shift+C", "Cmd+C")
// parses into a Chord, and a Chord decides whether a key event is that
// combination. Matching is exact on modifiers: Ctrl+P and Ctrl+Shift+P are
// different chords, which is what makes a conflict check meaningful.
type Chord struct {
	code xui.KeyCode // xui.KeyRune for printable chords, else the named key
	r    rune        // the lowercase hotkey rune when code == xui.KeyRune
	mods xui.Modifiers
}

// namedKeys are the non-rune keys a chord may name. Deliberately short:
// Enter, Esc, Tab and the arrows carry fixed meanings across the TUI, so a
// chord over them would fight the panes that own them.
var namedKeys = map[string]xui.KeyCode{
	"f1": xui.KeyF1, "f2": xui.KeyF2, "f3": xui.KeyF3, "f4": xui.KeyF4,
	"f5": xui.KeyF5, "f6": xui.KeyF6, "f7": xui.KeyF7, "f8": xui.KeyF8,
	"f9": xui.KeyF9, "f10": xui.KeyF10, "f11": xui.KeyF11, "f12": xui.KeyF12,
}

// keyNames is the display spelling per code, the reverse of namedKeys.
var keyNames = func() map[xui.KeyCode]string {
	m := make(map[xui.KeyCode]string, len(namedKeys))
	for name, code := range namedKeys {
		m[code] = strings.ToUpper(name)
	}
	return m
}()

// ParseChord parses a chord spelling: modifier names joined by + around one
// key, which is either a function key (F1–F12) or a single printable rune.
// Modifiers are Ctrl, Alt, Shift and Cmd (Super is a synonym), any case. A
// rune chord must carry Ctrl, Alt or Cmd — a bare or Shift-only rune is
// typing, and a table that could swallow typing is a trap.
func ParseChord(spelling string) (Chord, error) {
	s := strings.TrimSpace(spelling)
	if s == "" {
		return Chord{}, errors.New("empty chord")
	}
	parts := strings.Split(s, "+")
	// "Ctrl++" splits into a trailing empty part: the key is the plus itself.
	if n := len(parts); n >= 2 && parts[n-1] == "" && parts[n-2] == "" {
		parts = append(parts[:n-2], "+")
	}
	var c Chord
	for _, mod := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(mod)) {
		case "ctrl":
			c.mods |= xui.ModCtrl
		case "alt":
			c.mods |= xui.ModAlt
		case "shift":
			c.mods |= xui.ModShift
		case "cmd", "super":
			c.mods |= xui.ModSuper
		default:
			return Chord{}, fmt.Errorf("chord %q: unknown modifier %q", spelling, mod)
		}
	}
	key := strings.TrimSpace(parts[len(parts)-1])
	if code, ok := namedKeys[strings.ToLower(key)]; ok {
		c.code = code
		return c, nil
	}
	r, size := utf8.DecodeRuneInString(key)
	if size == 0 || size != len(key) || !unicode.IsPrint(r) || r == ' ' {
		return Chord{}, fmt.Errorf("chord %q: unknown key %q", spelling, key)
	}
	c.code = xui.KeyRune
	c.r = unicode.ToLower(r)
	if c.mods&(xui.ModCtrl|xui.ModAlt|xui.ModSuper) == 0 {
		return Chord{}, fmt.Errorf("chord %q: a printable key needs Ctrl, Alt or Cmd", spelling)
	}
	return c, nil
}

// String renders the canonical spelling: Ctrl+Alt+Shift+Cmd order, the key
// last, letters uppercase.
func (c Chord) String() string {
	var b strings.Builder
	for _, m := range [...]struct {
		mod  xui.Modifiers
		name string
	}{
		{xui.ModCtrl, "Ctrl"},
		{xui.ModAlt, "Alt"},
		{xui.ModShift, "Shift"},
		{xui.ModSuper, "Cmd"},
	} {
		if c.mods.Has(m.mod) {
			b.WriteString(m.name)
			b.WriteByte('+')
		}
	}
	if c.code == xui.KeyRune {
		b.WriteRune(unicode.ToUpper(c.r))
	} else {
		b.WriteString(keyNames[c.code])
	}
	return b.String()
}

// Match reports whether the key press is exactly this chord. Modifiers must
// match exactly, runes compare layout-aware and case-folded (HotkeyRune), and
// releases never match.
func (c Chord) Match(ev xui.KeyEvent) bool {
	if !ev.Press || ev.Mods != c.mods {
		return false
	}
	if c.code == xui.KeyRune {
		return ev.Code == xui.KeyRune && unicode.ToLower(ev.HotkeyRune()) == c.r
	}
	return ev.Code == c.code
}
