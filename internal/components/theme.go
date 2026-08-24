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

	// Message chrome ported from opencode.json (theme.secondary /
	// theme.backgroundPanel).
	Secondary       xui.Style // agent identity — user-message bar, turn marker ▣
	BackgroundPanel xui.Style // panel background behind user messages

	// Markdown prose and code-syntax roles ported from opencode.json
	// ("theme.markdown*" / "theme.syntax*"). Attributes ride with the color,
	// as upstream: strong is bold, emphasis italic, link labels underlined.
	Markdown MarkdownRoles
	Syntax   SyntaxRoles
}

// MarkdownRoles are the prose styling roles of the opencode palette.
type MarkdownRoles struct {
	Heading    xui.Style // headings — bold; the renderer underlines level 1
	Strong     xui.Style // **strong**
	Emph       xui.Style // *emphasis*
	InlineCode xui.Style // `inline code`
	Link       xui.Style // bare URLs
	LinkText   xui.Style // link labels
	BlockQuote xui.Style // quoted prose (the left rule stays Border)
	ListItem   xui.Style // bullet marker
	ListEnum   xui.Style // ordered-list marker
	CodeBlock  xui.Style // code text when no language highlights
}

// SyntaxRoles are the code-highlighting roles of the opencode palette.
type SyntaxRoles struct {
	Comment     xui.Style
	Keyword     xui.Style
	Function    xui.Style
	Variable    xui.Style
	String      xui.Style
	Number      xui.Style
	Type        xui.Style
	Operator    xui.Style
	Punctuation xui.Style
}

// ThemeNames lists builtin theme display names in picker order.
func ThemeNames() []string {
	return []string{"opencode", "opencode-light", "Dark", "Darcula", "Pink", "Terminal"}
}

// DefaultTheme returns the opencode dark palette — the CozyPhi house look.
func DefaultTheme() Theme { return OpencodeTheme() }

// OpencodeTheme ports the opencode TUI default theme, dark variant: warm
// orange primary, cool blue secondary, near-black grays. Values come from
// sst/opencode packages/tui/src/theme/assets/opencode.json; the upstream key
// each color came from is noted per field.
func OpencodeTheme() Theme {
	return Theme{
		Foreground:  xui.Style{Fg: xui.RGBColor(0xee, 0xee, 0xee)},                  // text
		Muted:       xui.Style{Fg: xui.RGBColor(0x80, 0x80, 0x80)},                  // textMuted
		Success:     xui.Style{Fg: xui.RGBColor(0x7f, 0xd8, 0x8f)},                  // success
		Accent:      xui.Style{Fg: xui.RGBColor(0xfa, 0xb2, 0x83), Underline: true}, // primary — links
		Warning:     xui.Style{Fg: xui.RGBColor(0xf5, 0xa7, 0x42)},                  // warning
		Destructive: xui.Style{Fg: xui.RGBColor(0xe0, 0x6c, 0x75)},                  // error
		Border:      xui.Style{Fg: xui.RGBColor(0x48, 0x48, 0x48)},                  // border
		ToolName:    xui.Style{Fg: xui.RGBColor(0x5c, 0x9c, 0xf5)},                  // secondary
		SelectionBg: xui.Style{Bg: xui.RGBColor(0xfa, 0xb2, 0x83)},                  // primary bar
		SelectionFg: xui.Style{Fg: xui.RGBColor(0x0a, 0x0a, 0x0a), Bold: true},      // selectedForeground → background
		Keybind:     xui.Style{Fg: xui.RGBColor(0x5c, 0x9c, 0xf5), Bold: true},      // secondary
		Command:     xui.Style{Fg: xui.RGBColor(0x5c, 0x9c, 0xf5)},                  // secondary
		Secondary:   xui.Style{Fg: xui.RGBColor(0x5c, 0x9c, 0xf5)},                  // secondary
		BackgroundPanel: xui.Style{
			Bg: xui.RGBColor(0x14, 0x14, 0x14), // backgroundPanel — darkStep2
		},
		Markdown: MarkdownRoles{ // theme.markdown*
			Heading:    xui.Style{Fg: xui.RGBColor(0x9d, 0x7c, 0xd8), Bold: true},      // darkAccent
			Strong:     xui.Style{Fg: xui.RGBColor(0xf5, 0xa7, 0x42), Bold: true},      // darkOrange
			Emph:       xui.Style{Fg: xui.RGBColor(0xe5, 0xc0, 0x7b), Italic: true},    // darkYellow
			InlineCode: xui.Style{Fg: xui.RGBColor(0x7f, 0xd8, 0x8f)},                  // darkGreen
			Link:       xui.Style{Fg: xui.RGBColor(0xfa, 0xb2, 0x83), Underline: true}, // darkStep9
			LinkText:   xui.Style{Fg: xui.RGBColor(0x56, 0xb6, 0xc2), Underline: true}, // darkCyan
			BlockQuote: xui.Style{Fg: xui.RGBColor(0xe5, 0xc0, 0x7b), Italic: true},    // darkYellow
			ListItem:   xui.Style{Fg: xui.RGBColor(0xfa, 0xb2, 0x83)},                  // darkStep9
			ListEnum:   xui.Style{Fg: xui.RGBColor(0x56, 0xb6, 0xc2)},                  // darkCyan
			CodeBlock:  xui.Style{Fg: xui.RGBColor(0xee, 0xee, 0xee)},                  // darkStep12
		},
		Syntax: SyntaxRoles{ // theme.syntax*
			Comment:     xui.Style{Fg: xui.RGBColor(0x80, 0x80, 0x80)}, // darkStep11
			Keyword:     xui.Style{Fg: xui.RGBColor(0x9d, 0x7c, 0xd8)}, // darkAccent
			Function:    xui.Style{Fg: xui.RGBColor(0xfa, 0xb2, 0x83)}, // darkStep9
			Variable:    xui.Style{Fg: xui.RGBColor(0xe0, 0x6c, 0x75)}, // darkRed
			String:      xui.Style{Fg: xui.RGBColor(0x7f, 0xd8, 0x8f)}, // darkGreen
			Number:      xui.Style{Fg: xui.RGBColor(0xf5, 0xa7, 0x42)}, // darkOrange
			Type:        xui.Style{Fg: xui.RGBColor(0xe5, 0xc0, 0x7b)}, // darkYellow
			Operator:    xui.Style{Fg: xui.RGBColor(0x56, 0xb6, 0xc2)}, // darkCyan
			Punctuation: xui.Style{Fg: xui.RGBColor(0xee, 0xee, 0xee)}, // darkStep12
		},
	}
}

// OpencodeLightTheme is the light variant of the opencode palette: blue
// primary, violet secondary, warm amber accent.
func OpencodeLightTheme() Theme {
	return Theme{
		Foreground:  xui.Style{Fg: xui.RGBColor(0x1a, 0x1a, 0x1a)},
		Muted:       xui.Style{Fg: xui.RGBColor(0x8a, 0x8a, 0x8a)},
		Success:     xui.Style{Fg: xui.RGBColor(0x3d, 0x9a, 0x57)},
		Accent:      xui.Style{Fg: xui.RGBColor(0x3b, 0x7d, 0xd8), Underline: true},
		Warning:     xui.Style{Fg: xui.RGBColor(0xd6, 0x8c, 0x27)},
		Destructive: xui.Style{Fg: xui.RGBColor(0xd1, 0x38, 0x3d)},
		Border:      xui.Style{Fg: xui.RGBColor(0xb8, 0xb8, 0xb8)},
		ToolName:    xui.Style{Fg: xui.RGBColor(0x7b, 0x5b, 0xb6)},
		SelectionBg: xui.Style{Bg: xui.RGBColor(0x3b, 0x7d, 0xd8)},
		SelectionFg: xui.Style{Fg: xui.RGBColor(0xff, 0xff, 0xff), Bold: true},
		Keybind:     xui.Style{Fg: xui.RGBColor(0x7b, 0x5b, 0xb6), Bold: true},
		Command:     xui.Style{Fg: xui.RGBColor(0x7b, 0x5b, 0xb6)},
		Secondary:   xui.Style{Fg: xui.RGBColor(0x7b, 0x5b, 0xb6)}, // secondary
		BackgroundPanel: xui.Style{
			Bg: xui.RGBColor(0xfa, 0xfa, 0xfa), // backgroundPanel — lightStep2
		},
		Markdown: MarkdownRoles{
			Heading:    xui.Style{Fg: xui.RGBColor(0xd6, 0x8c, 0x27), Bold: true},      // lightAccent
			Strong:     xui.Style{Fg: xui.RGBColor(0xd6, 0x8c, 0x27), Bold: true},      // lightOrange
			Emph:       xui.Style{Fg: xui.RGBColor(0xb0, 0x85, 0x1f), Italic: true},    // lightYellow
			InlineCode: xui.Style{Fg: xui.RGBColor(0x3d, 0x9a, 0x57)},                  // lightGreen
			Link:       xui.Style{Fg: xui.RGBColor(0x3b, 0x7d, 0xd8), Underline: true}, // lightStep9
			LinkText:   xui.Style{Fg: xui.RGBColor(0x31, 0x87, 0x95), Underline: true}, // lightCyan
			BlockQuote: xui.Style{Fg: xui.RGBColor(0xb0, 0x85, 0x1f), Italic: true},    // lightYellow
			ListItem:   xui.Style{Fg: xui.RGBColor(0x3b, 0x7d, 0xd8)},                  // lightStep9
			ListEnum:   xui.Style{Fg: xui.RGBColor(0x31, 0x87, 0x95)},                  // lightCyan
			CodeBlock:  xui.Style{Fg: xui.RGBColor(0x1a, 0x1a, 0x1a)},                  // lightStep12
		},
		Syntax: SyntaxRoles{
			Comment:     xui.Style{Fg: xui.RGBColor(0x8a, 0x8a, 0x8a)}, // lightStep11
			Keyword:     xui.Style{Fg: xui.RGBColor(0xd6, 0x8c, 0x27)}, // lightAccent
			Function:    xui.Style{Fg: xui.RGBColor(0x3b, 0x7d, 0xd8)}, // lightStep9
			Variable:    xui.Style{Fg: xui.RGBColor(0xd1, 0x38, 0x3d)}, // lightRed
			String:      xui.Style{Fg: xui.RGBColor(0x3d, 0x9a, 0x57)}, // lightGreen
			Number:      xui.Style{Fg: xui.RGBColor(0xd6, 0x8c, 0x27)}, // lightOrange
			Type:        xui.Style{Fg: xui.RGBColor(0xb0, 0x85, 0x1f)}, // lightYellow
			Operator:    xui.Style{Fg: xui.RGBColor(0x31, 0x87, 0x95)}, // lightCyan
			Punctuation: xui.Style{Fg: xui.RGBColor(0x1a, 0x1a, 0x1a)}, // lightStep12
		},
	}
}

// legacyMarkdownAndSyntax keeps the pre-role look of the bundled themes:
// inline and plain code warning, markers and quotes muted, link labels
// accent, keywords the tool blue. Only the opencode themes carry the upstream
// markdown palette.
func legacyMarkdownAndSyntax(th Theme) (MarkdownRoles, SyntaxRoles) {
	md := MarkdownRoles{
		Heading:    xui.Style{Fg: th.Accent.Fg, Bold: true},
		Strong:     xui.Style{Fg: th.Warning.Fg, Bold: true},
		Emph:       xui.Style{Fg: th.Warning.Fg, Italic: true},
		InlineCode: xui.Style{Fg: th.Warning.Fg},
		Link:       xui.Style{Fg: th.Accent.Fg, Underline: true},
		LinkText:   xui.Style{Fg: th.Accent.Fg, Underline: true},
		BlockQuote: xui.Style{Fg: th.Muted.Fg, Italic: true},
		ListItem:   xui.Style{Fg: th.Muted.Fg},
		ListEnum:   xui.Style{Fg: th.Muted.Fg},
		CodeBlock:  xui.Style{Fg: th.Warning.Fg},
	}
	sy := SyntaxRoles{
		Comment:     xui.Style{Fg: th.Muted.Fg},
		Keyword:     xui.Style{Fg: th.ToolName.Fg, Bold: true},
		Function:    xui.Style{Fg: th.Accent.Fg},
		Variable:    xui.Style{Fg: th.Destructive.Fg},
		String:      xui.Style{Fg: th.Success.Fg},
		Number:      xui.Style{Fg: th.Warning.Fg},
		Type:        xui.Style{Fg: th.Accent.Fg},
		Operator:    xui.Style{Fg: th.Foreground.Fg},
		Punctuation: xui.Style{Fg: th.Foreground.Fg},
	}
	return md, sy
}

// legacyChrome pins the pre-opencode message chrome: legacy themes have no
// agent palette, so the identity color stays Accent and the user-message
// panel paints the terminal default background (visually no panel).
func legacyChrome(th *Theme) {
	th.Secondary = th.Accent
	th.BackgroundPanel = xui.Style{Bg: xui.DefaultColor()}
}

// DarkTheme is the fixed RGB dark palette ("Dark").
func DarkTheme() Theme {
	th := Theme{
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
	th.Markdown, th.Syntax = legacyMarkdownAndSyntax(th)
	legacyChrome(&th)
	return th
}

// DarculaTheme follows IntelliJ IDEA Darcula (warm orange accents, cool text).
func DarculaTheme() Theme {
	th := Theme{
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
	th.Markdown, th.Syntax = legacyMarkdownAndSyntax(th)
	legacyChrome(&th)
	return th
}

// PinkTheme is a sakura blush palette — warm pink accents, soft and readable.
func PinkTheme() Theme {
	th := Theme{
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
	th.Markdown, th.Syntax = legacyMarkdownAndSyntax(th)
	legacyChrome(&th)
	return th
}

// TerminalTheme follows the terminal ANSI / default colors ("Terminal").
func TerminalTheme() Theme {
	th := Theme{
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
	th.Markdown, th.Syntax = legacyMarkdownAndSyntax(th)
	legacyChrome(&th)
	return th
}

// ThemeByName resolves a theme by display name (case-insensitive).
func ThemeByName(name string) (Theme, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "opencode":
		return OpencodeTheme(), true
	case "opencode-light", "opencode light":
		return OpencodeLightTheme(), true
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
