// Package xui is the high-level terminal UI engine.
//
// Subpackages follow Google-style Go layout:
//
//	cell     — Cell, Style, Color, grapheme width
//	screen   — double-buffered Screen / Window
//	render   — differential ANSI renderer
//	term     — raw TTY I/O
//	input    — parser and events
package xui

import (
	"github.com/pulseaiclub/xui/cell"
	"github.com/pulseaiclub/xui/input"
	"github.com/pulseaiclub/xui/render"
	"github.com/pulseaiclub/xui/screen"
	"github.com/pulseaiclub/xui/term"
)

// Core type aliases so applications can import a single package for the engine API.
type (
	Cell      = cell.Cell
	Style     = cell.Style
	Color     = cell.Color
	ColorKind = cell.ColorKind
	Hyperlink = cell.Hyperlink
	DirtyCell = cell.DirtyCell

	WidthMethod = cell.WidthMethod

	Screen = screen.Screen
	Window = screen.Window

	Renderer = render.Renderer
	Caps     = render.Caps

	TTY = term.TTY

	Event       = input.Event
	KeyEvent    = input.KeyEvent
	MouseEvent  = input.MouseEvent
	ResizeEvent = input.ResizeEvent
	PasteEvent  = input.PasteEvent
	FocusEvent  = input.FocusEvent
	CapEvent    = input.CapEvent
	TickEvent   = input.TickEvent
	Modifiers   = input.Modifiers
	KeyCode     = input.KeyCode
	MouseButton = input.MouseButton
	MouseAction = input.MouseAction
	CapKind     = input.CapKind
	Parser      = input.Parser
)

// Re-exported constructors / helpers.
var (
	EmptyCell     = cell.EmptyCell
	DefaultColor  = cell.DefaultColor
	IndexedColor  = cell.IndexedColor
	RGBColor      = cell.RGBColor
	StringWidth   = cell.StringWidth
	FirstGrapheme = cell.FirstGrapheme

	NewScreen = screen.NewScreen
	NewWindow = screen.NewWindow

	NewRenderer = render.NewRenderer

	OpenTTY = term.OpenTTY

	NewParser = input.NewParser
)

const (
	ColorDefault = cell.ColorDefault
	ColorIndex   = cell.ColorIndex
	ColorRGB     = cell.ColorRGB

	WidthUnicode = cell.WidthUnicode
	WidthWCWidth = cell.WidthWCWidth

	ModShift = input.ModShift
	ModAlt   = input.ModAlt
	ModCtrl  = input.ModCtrl
	ModSuper = input.ModSuper

	KeyNone      = input.KeyNone
	KeyRune      = input.KeyRune
	KeyEnter     = input.KeyEnter
	KeyEscape    = input.KeyEscape
	KeyBackspace = input.KeyBackspace
	KeyTab       = input.KeyTab
	KeyUp        = input.KeyUp
	KeyDown      = input.KeyDown
	KeyLeft      = input.KeyLeft
	KeyRight     = input.KeyRight
	KeyHome      = input.KeyHome
	KeyEnd       = input.KeyEnd
	KeyInsert    = input.KeyInsert
	KeyDelete    = input.KeyDelete
	KeyPageUp    = input.KeyPageUp
	KeyPageDown  = input.KeyPageDown
	KeyF1        = input.KeyF1
	KeyF2        = input.KeyF2
	KeyF3        = input.KeyF3
	KeyF4        = input.KeyF4
	KeyF5        = input.KeyF5
	KeyF6        = input.KeyF6
	KeyF7        = input.KeyF7
	KeyF8        = input.KeyF8
	KeyF9        = input.KeyF9
	KeyF10       = input.KeyF10
	KeyF11       = input.KeyF11
	KeyF12       = input.KeyF12

	MouseNone      = input.MouseNone
	MouseLeft      = input.MouseLeft
	MouseMiddle    = input.MouseMiddle
	MouseRight     = input.MouseRight
	MouseWheelUp   = input.MouseWheelUp
	MouseWheelDown = input.MouseWheelDown
	MousePress     = input.MousePress
	MouseRelease   = input.MouseRelease
	MouseMotion    = input.MouseMotion
	MouseDrag      = input.MouseDrag

	CapDA1       = input.CapDA1
	CapKittyKB   = input.CapKittyKB
	CapDECRQM    = input.CapDECRQM
	CapXTVersion = input.CapXTVersion
)
