package input

import "time"

// Event is a tagged terminal input or system event.
type Event interface {
	isEvent()
}

// KeyEvent is a keyboard press or release.
type KeyEvent struct {
	Code    KeyCode
	Rune    rune
	AltRune rune // kitty "alternate key code": US-layout key; zero when absent
	Text    string
	Mods    Modifiers
	Press   bool // true = press, false = release
}

func (KeyEvent) isEvent() {}

// Modifiers are key modifier flags.
type Modifiers uint8

const (
	ModShift Modifiers = 1 << iota
	ModAlt
	ModCtrl
	ModSuper
)

// Has reports whether m contains the given modifiers.
func (m Modifiers) Has(want Modifiers) bool {
	return m&want == want
}

// KeyCode identifies special keys.
type KeyCode uint16

const (
	KeyNone KeyCode = iota
	KeyRune         // printable; see KeyEvent.Rune
	KeyEnter
	KeyEscape
	KeyBackspace
	KeyTab
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyInsert
	KeyDelete
	KeyPageUp
	KeyPageDown
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
)

// Matches reports a loose key match (code/rune + required mods). Rune keys
// match on the hotkey rune, so a Cyrillic layout still matches its QWERTY key.
func (k KeyEvent) Matches(code KeyCode, r rune, mods Modifiers) bool {
	if k.Mods&mods != mods {
		return false
	}
	if code == KeyRune {
		return k.Code == KeyRune && k.HotkeyRune() == r
	}
	return k.Code == code
}

// HotkeyRune returns the rune to compare against hotkey bindings: the kitty
// alternate (US-layout) key when the terminal reported one, else the rune
// with its non-Latin layout mapping reversed. Text entry must keep using Rune.
func (k KeyEvent) HotkeyRune() rune {
	if k.AltRune != 0 {
		return k.AltRune
	}
	return layoutLatin(k.Rune)
}

// CtrlC reports whether this is Ctrl+C.
func (k KeyEvent) CtrlC() bool {
	return (k.Code == KeyRune && k.HotkeyRune() == 'c' && k.Mods.Has(ModCtrl)) ||
		(k.Code == KeyRune && k.Rune == 0x03)
}

// MouseEvent is a mouse report.
type MouseEvent struct {
	X, Y   int
	Button MouseButton
	Action MouseAction
	Mods   Modifiers
	// Wheel is the number of wheel notches for MouseWheelUp/Down. Zero means 1.
	// App event loops may coalesce rapid flicks into a single event with Wheel>1.
	Wheel int
}

func (MouseEvent) isEvent() {}

// MouseButton identifies a mouse button.
type MouseButton uint8

const (
	MouseNone MouseButton = iota
	MouseLeft
	MouseMiddle
	MouseRight
	MouseWheelUp
	MouseWheelDown
)

// MouseAction is press/release/move/drag.
type MouseAction uint8

const (
	MousePress MouseAction = iota
	MouseRelease
	MouseMotion
	MouseDrag
)

// ResizeEvent is a terminal size change.
type ResizeEvent struct {
	Cols, Rows     int
	XPixel, YPixel int
}

func (ResizeEvent) isEvent() {}

// PasteEvent is bracketed paste content.
type PasteEvent struct {
	Text string
}

func (PasteEvent) isEvent() {}

// FocusEvent reports terminal focus gain/loss.
type FocusEvent struct {
	Focused bool
}

func (FocusEvent) isEvent() {}

// CapEvent is an internal capability probe response (not delivered to apps).
type CapEvent struct {
	Kind CapKind
	Data string
}

func (CapEvent) isEvent() {}

// CapKind classifies capability responses.
type CapKind int

const (
	CapDA1 CapKind = iota
	CapKittyKB
	CapDECRQM
	CapXTVersion
)

// TickEvent is a timer tick used by the components runtime.
type TickEvent struct {
	At time.Time
}

func (TickEvent) isEvent() {}
