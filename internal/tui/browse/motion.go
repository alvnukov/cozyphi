// Package browse holds the shared state machines behind every list-shaped
// surface of the TUI: the motion dialect, the cursor and the scroller that
// follow it, and the armed y/n confirmation. Panes stay in charge of their
// rows and their actions; everything the panes have in common lives here,
// and the contract is written down in internal/tui/DESIGN.md.
package browse

import "github.com/pulseaiclub/xui"

// Op names what a motion does; N carries how far or where.
type Op int

const (
	// OpNone is a consumed keypress with nothing to apply yet: a count
	// digit, or the first g of a gg.
	OpNone Op = iota
	// OpStep moves N rows; negative is up.
	OpStep
	// OpHalfPage moves N half-screens; negative is up.
	OpHalfPage
	// OpPage moves N screens; negative is up.
	OpPage
	// OpTop jumps to the first row.
	OpTop
	// OpBottom jumps to the last row.
	OpBottom
	// OpIndex jumps to row N, counted from one (vim's 12G).
	OpIndex
)

// Motion is one parsed movement, ready for a Cursor or a Scroller to apply.
type Motion struct {
	Op Op
	N  int
}

// Motions turns pressed keys into motions. It owns the pending vim-style
// input — a count prefix ("3j") and a half-typed "gg" — which any key
// outside the dialect clears.
type Motions struct {
	count    int
	pendingG bool
}

// Reset drops pending input; call it when the pane changes what it shows.
func (m *Motions) Reset() { m.count, m.pendingG = 0, false }

// take consumes the pending count, defaulting to one.
func (m *Motions) take() int {
	n := max(m.count, 1)
	m.Reset()
	return n
}

// Key parses one pressed key. ok reports that the key belongs to the
// dialect and is consumed; any other key clears pending input and stays
// with the caller. Modified arrows are never motions — Shift+↑↓ and
// friends are pane-specific by contract.
func (m *Motions) Key(e xui.KeyEvent) (Motion, bool) {
	if e.Code == xui.KeyRune {
		return m.runeKey(e)
	}
	if e.Mods != 0 {
		m.Reset()
		return Motion{}, false
	}
	switch e.Code {
	case xui.KeyUp:
		m.Reset()
		return Motion{Op: OpStep, N: -1}, true
	case xui.KeyDown:
		m.Reset()
		return Motion{Op: OpStep, N: 1}, true
	case xui.KeyPageUp:
		m.Reset()
		return Motion{Op: OpPage, N: -1}, true
	case xui.KeyPageDown:
		m.Reset()
		return Motion{Op: OpPage, N: 1}, true
	case xui.KeyHome:
		m.Reset()
		return Motion{Op: OpTop}, true
	case xui.KeyEnd:
		m.Reset()
		return Motion{Op: OpBottom}, true
	}
	m.Reset()
	return Motion{}, false
}

// runeKey decodes the letter half of the dialect. Shift+letter arrives as
// a capital rune with ModShift, so the modifier guard must let G through
// instead of swallowing it.
func (m *Motions) runeKey(e xui.KeyEvent) (Motion, bool) {
	r := e.HotkeyRune()
	switch {
	case e.Mods == xui.ModCtrl && (r == 'u' || r == 'd'):
		n := m.take()
		if r == 'u' {
			n = -n
		}
		return Motion{Op: OpHalfPage, N: n}, true
	case e.Mods == xui.ModShift && r == 'G':
		return m.bigG(), true
	case e.Mods != 0:
		m.Reset()
		return Motion{}, false
	case r >= '0' && r <= '9':
		m.count = m.count*10 + int(r-'0')
		m.pendingG = false
		return Motion{}, true
	case r == 'j':
		return Motion{Op: OpStep, N: m.take()}, true
	case r == 'k':
		return Motion{Op: OpStep, N: -m.take()}, true
	case r == 'g':
		if m.pendingG {
			m.Reset()
			return Motion{Op: OpTop}, true
		}
		m.pendingG = true
		return Motion{}, true
	case r == 'G':
		return m.bigG(), true
	}
	m.Reset()
	return Motion{}, false
}

// bigG jumps to the bottom, or with a count to that row ("12G").
func (m *Motions) bigG() Motion {
	if m.count > 0 {
		return Motion{Op: OpIndex, N: m.take()}
	}
	m.Reset()
	return Motion{Op: OpBottom}
}

// WheelStep is how many rows one wheel notch moves — the terminal-emulator
// convention, and fast enough to cross a long list.
const WheelStep = 3

// Wheel parses a mouse event into a scroll motion, WheelStep rows per
// notch.
func Wheel(e xui.MouseEvent) (Motion, bool) {
	notches := max(e.Wheel, 1)
	switch e.Button {
	case xui.MouseWheelUp:
		return Motion{Op: OpStep, N: -notches * WheelStep}, true
	case xui.MouseWheelDown:
		return Motion{Op: OpStep, N: notches * WheelStep}, true
	}
	return Motion{}, false
}
