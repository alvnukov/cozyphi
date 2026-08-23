package components

import (
	"strings"

	"github.com/pulseaiclub/xui"
)

// Theme holds semantic colors for transcript chrome.
type Theme struct {
	Foreground  xui.Style
	Muted       xui.Style
	Success     xui.Style
	Accent      xui.Style // links / "Show more"
	Warning     xui.Style // inline highlights / palette title
	Destructive xui.Style
	Border      xui.Style
	ToolName    xui.Style
	// Command palette.
	SelectionBg xui.Style // yellow bar behind selected row
	SelectionFg xui.Style // black text on selection
	Keybind     xui.Style // shortcut hints (Ctrl g)
	Command     xui.Style // command accent
}

// ThemeNames lists builtin theme display names in picker order.
func ThemeNames() []string {
	return []string{"Dark", "Darcula", "Pink", "Terminal"}
}

// DefaultTheme returns the terminal default palette (ANSI / terminal colors).
func DefaultTheme() Theme { return TerminalTheme() }

// DarkTheme is the fixed RGB dark palette ("Dark").
func DarkTheme() Theme {
	return Theme{
		Foreground:  xui.Style{Fg: xui.DefaultColor()},
		Muted:       xui.Style{Fg: xui.IndexedColor(245)},
		Success:     xui.Style{Fg: xui.RGBColor(0x7d, 0xc3, 0xa0), Bold: true},
		Accent:      xui.Style{Fg: xui.RGBColor(0xc4, 0x8a, 0xd9), Underline: true},
		Warning:     xui.Style{Fg: xui.RGBColor(0xe5, 0xc0, 0x7b)},
		Destructive: xui.Style{Fg: xui.RGBColor(0xe0, 0x6c, 0x75)},
		Border:      xui.Style{Fg: xui.IndexedColor(240)},
		ToolName:    xui.Style{Fg: xui.RGBColor(0x7d, 0xc3, 0xff)},
		SelectionBg: xui.Style{Bg: xui.RGBColor(0xe5, 0xc0, 0x7b)},
		SelectionFg: xui.Style{Fg: xui.RGBColor(0x00, 0x00, 0x00), Bold: true},
		Keybind:     xui.Style{Fg: xui.RGBColor(0x61, 0xaf, 0xef), Bold: true},
		Command:     xui.Style{Fg: xui.RGBColor(0xe5, 0xc0, 0x7b)},
	}
}

// DarculaTheme follows IntelliJ IDEA Darcula (warm orange accents, cool text).
func DarculaTheme() Theme {
	return Theme{
		Foreground:  xui.Style{Fg: xui.RGBColor(0xa9, 0xb7, 0xc6)},
		Muted:       xui.Style{Fg: xui.RGBColor(0x80, 0x80, 0x80), Dim: true},
		Success:     xui.Style{Fg: xui.RGBColor(0x6a, 0x87, 0x59), Bold: true},
		Accent:      xui.Style{Fg: xui.RGBColor(0x58, 0x9d, 0xf6), Underline: true},
		Warning:     xui.Style{Fg: xui.RGBColor(0xcc, 0x78, 0x32)},
		Destructive: xui.Style{Fg: xui.RGBColor(0xff, 0x6b, 0x68)},
		Border:      xui.Style{Fg: xui.RGBColor(0x55, 0x55, 0x55)},
		ToolName:    xui.Style{Fg: xui.RGBColor(0x68, 0x97, 0xbb)},
		SelectionBg: xui.Style{Bg: xui.RGBColor(0x21, 0x42, 0x83)},
		SelectionFg: xui.Style{Fg: xui.RGBColor(0xff, 0xff, 0xff), Bold: true},
		Keybind:     xui.Style{Fg: xui.RGBColor(0x58, 0x9d, 0xf6), Bold: true},
		Command:     xui.Style{Fg: xui.RGBColor(0xcc, 0x78, 0x32)},
	}
}

// PinkTheme is a sakura blush palette — warm pink accents, soft and readable.
func PinkTheme() Theme {
	return Theme{
		Foreground:  xui.Style{Fg: xui.DefaultColor()},
		Muted:       xui.Style{Fg: xui.RGBColor(0xc8, 0xa0, 0xb4), Dim: true},
		Success:     xui.Style{Fg: xui.RGBColor(0x9e, 0xd4, 0xb8), Bold: true},
		Accent:      xui.Style{Fg: xui.RGBColor(0xff, 0x9e, 0xc8), Underline: true},
		Warning:     xui.Style{Fg: xui.RGBColor(0xff, 0xb8, 0x9a)},
		Destructive: xui.Style{Fg: xui.RGBColor(0xf0, 0x6a, 0x8a)},
		Border:      xui.Style{Fg: xui.RGBColor(0x8a, 0x5a, 0x70)},
		ToolName:    xui.Style{Fg: xui.RGBColor(0xf0, 0xa8, 0xd0)},
		SelectionBg: xui.Style{Bg: xui.RGBColor(0xff, 0x9e, 0xc0)},
		SelectionFg: xui.Style{Fg: xui.RGBColor(0x2a, 0x10, 0x1c), Bold: true},
		Keybind:     xui.Style{Fg: xui.RGBColor(0xff, 0x8f, 0xb8), Bold: true},
		Command:     xui.Style{Fg: xui.RGBColor(0xff, 0x7a, 0xad)},
	}
}

// TerminalTheme follows the terminal ANSI / default colors ("Terminal").
func TerminalTheme() Theme {
	return Theme{
		Foreground:  xui.Style{Fg: xui.DefaultColor()},
		Muted:       xui.Style{Fg: xui.IndexedColor(8)},
		Success:     xui.Style{Fg: xui.IndexedColor(2), Bold: true},
		Accent:      xui.Style{Fg: xui.IndexedColor(5), Underline: true},
		Warning:     xui.Style{Fg: xui.IndexedColor(3)},
		Destructive: xui.Style{Fg: xui.IndexedColor(1)},
		Border:      xui.Style{Fg: xui.IndexedColor(8)},
		ToolName:    xui.Style{Fg: xui.IndexedColor(4)},
		SelectionBg: xui.Style{Bg: xui.IndexedColor(3)},
		SelectionFg: xui.Style{Fg: xui.IndexedColor(0), Bold: true},
		Keybind:     xui.Style{Fg: xui.IndexedColor(4), Bold: true},
		Command:     xui.Style{Fg: xui.IndexedColor(3)},
	}
}

// ThemeByName resolves a theme by display name (case-insensitive).
func ThemeByName(name string) (Theme, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "dark":
		return DarkTheme(), true
	case "darcula", "dura":
		return DarculaTheme(), true
	case "pink", "sakura":
		return PinkTheme(), true
	case "terminal":
		return TerminalTheme(), true
	default:
		return Theme{}, false
	}
}
