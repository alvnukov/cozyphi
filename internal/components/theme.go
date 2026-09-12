package components

import (
	"strings"

	"github.com/pulseaiclub/xui"
)

// Theme holds semantic colors for transcript chrome.
type Theme struct {
	// Name is the display name the picker and /theme know this palette by.
	// It travels with the palette so a surface can say which one it is
	// painting with without keeping a second copy of the answer in step.
	Name        string
	Foreground  xui.Style
	Background  xui.Style // root canvas fill; DefaultColor = follow the terminal
	Muted       xui.Style
	Success     xui.Style
	Accent      xui.Style // links / "Show more"
	Warning     xui.Style // inline highlights / palette title
	Violet      xui.Style // useplan mode accent
	Destructive xui.Style
	Border      xui.Style
	ToolName    xui.Style
	// Command palette.
	SelectionBg xui.Style // yellow bar behind selected row
	SelectionFg xui.Style // black text on selection
	Keybind     xui.Style // shortcut hints (Ctrl g)
	Command     xui.Style // command accent

	// Mention / slash picker selection (kept distinct from the palette bar)
	// and the transcript block-copy highlight.
	PickerSelectionBg    xui.Style // bar behind the selected picker row
	PickerSelectionFg    xui.Style // label on the picker selection bar
	PickerSelectionMuted xui.Style // description on the picker selection bar
	BlockHighlight       xui.Style // background of the copy-selected transcript block

	// Message chrome ported from opencode.json (theme.secondary /
	// theme.backgroundPanel / theme.backgroundElement).
	Secondary         xui.Style // agent identity — user-message bar, turn marker ▣
	BackgroundPanel   xui.Style // panel background behind user messages
	BackgroundElement xui.Style // editor surfaces (composer input panel)

	// Diff rows: DiffAdd / DiffRemove color the +/- marker column; palettes
	// that ship no pair of their own inherit the Success / Destructive text
	// colors. DiffAddedBg / DiffRemovedBg are the full-row backdrops, dim
	// enough that highlighted code still reads on top of them. Context rows
	// keep BackgroundPanel, so the three kinds differ by tint alone and the
	// block stays one card.
	DiffAdd       xui.Style // "+" marker of an added diff row
	DiffRemove    xui.Style // "-" marker of a removed diff row
	DiffAddedBg   xui.Style // full-row background behind an added diff row
	DiffRemovedBg xui.Style // full-row background behind a removed diff row

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
	return []string{"opencode", "Light (VS)", "Dark", "Darcula", "Pink", "Terminal"}
}

// DefaultTheme returns the opencode dark palette — the CozyPhi house look.
func DefaultTheme() Theme { return OpencodeTheme() }

// OpencodeTheme ports the opencode TUI default theme, dark variant: warm
// orange primary, cool blue secondary, near-black grays. Values come from
// sst/opencode packages/tui/src/theme/assets/opencode.json; the upstream key
// each color came from is noted per field.
func OpencodeTheme() Theme {
	th := Theme{
		Name:        "opencode",
		Foreground:  xui.Style{Fg: xui.RGBColor(0xee, 0xee, 0xee)},                  // text
		Muted:       xui.Style{Fg: xui.RGBColor(0x80, 0x80, 0x80)},                  // textMuted
		Success:     xui.Style{Fg: xui.RGBColor(0x7f, 0xd8, 0x8f)},                  // success
		Accent:      xui.Style{Fg: xui.RGBColor(0xfa, 0xb2, 0x83), Underline: true}, // primary — links
		Warning:     xui.Style{Fg: xui.RGBColor(0xf5, 0xa7, 0x42)},                  // warning
		Violet:      xui.Style{Fg: xui.RGBColor(0x9d, 0x7c, 0xd8)},                  // darkAccent
		Destructive: xui.Style{Fg: xui.RGBColor(0xe0, 0x6c, 0x75)},                  // error
		Border:      xui.Style{Fg: xui.RGBColor(0x48, 0x48, 0x48)},                  // border
		ToolName:    xui.Style{Fg: xui.RGBColor(0x5c, 0x9c, 0xf5)},                  // secondary
		SelectionBg: xui.Style{Bg: xui.RGBColor(0xfa, 0xb2, 0x83)},                  // primary bar
		SelectionFg: xui.Style{Fg: xui.RGBColor(0x0a, 0x0a, 0x0a), Bold: true},      // selectedForeground → background
		Keybind:     xui.Style{Fg: xui.RGBColor(0x5c, 0x9c, 0xf5), Bold: true},      // secondary
		Command:     xui.Style{Fg: xui.RGBColor(0x5c, 0x9c, 0xf5)},                  // secondary
		// Soft blue picker bar — deliberately distinct from the palette's
		// primary yellow; the block highlight is a dark olive tint.
		PickerSelectionBg:    xui.Style{Bg: xui.RGBColor(0x3a, 0x5a, 0x7a)},
		PickerSelectionFg:    xui.Style{Fg: xui.RGBColor(0xe0, 0xf0, 0xff), Bold: true},
		PickerSelectionMuted: xui.Style{Fg: xui.RGBColor(0xb0, 0xc8, 0xe0)},
		BlockHighlight:       xui.Style{Bg: xui.RGBColor(0x2a, 0x2e, 0x24)},
		Secondary:            xui.Style{Fg: xui.RGBColor(0x5c, 0x9c, 0xf5)}, // secondary
		Background:           xui.Style{Bg: xui.RGBColor(0x0a, 0x0a, 0x0a)}, // background, darkStep1
		BackgroundPanel: xui.Style{
			Bg: xui.RGBColor(0x14, 0x14, 0x14), // backgroundPanel — darkStep2
		},
		BackgroundElement: xui.Style{
			Bg: xui.RGBColor(0x1e, 0x1e, 0x1e), // backgroundElement — darkStep3
		},
		// darkGreen / darkRed carried down to a backdrop (≈18% over
		// backgroundPanel), so a full-width diff row reads as green or red
		// without drowning the syntax colors on it.
		DiffAddedBg:   xui.Style{Bg: xui.RGBColor(0x1e, 0x33, 0x26)},
		DiffRemovedBg: xui.Style{Bg: xui.RGBColor(0x3a, 0x23, 0x26)},
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
	inheritDiffMarkers(&th)
	return th
}

// VSLightTheme is the Visual Studio light palette ("Light (VS)"): the
// classic VS C/C++ syntax colors on paper whites, one selection blue for
// every bar. It replaces the earlier opencode-light variant; the old names
// still resolve in ThemeByName so saved configs keep working.
func VSLightTheme() Theme {
	return Theme{
		Name:        "Light (VS)",
		Foreground:  xui.Style{Fg: xui.RGBColor(0x1F, 0x1F, 0x1F)},
		Muted:       xui.Style{Fg: xui.RGBColor(0x66, 0x66, 0x66)},
		Success:     xui.Style{Fg: xui.RGBColor(0x10, 0x7C, 0x10)},
		Accent:      xui.Style{Fg: xui.RGBColor(0x00, 0x78, 0xD4), Underline: true},
		Warning:     xui.Style{Fg: xui.RGBColor(0x8A, 0x5A, 0x00)},
		Violet:      xui.Style{Fg: xui.RGBColor(0x68, 0x21, 0x7A)},
		Destructive: xui.Style{Fg: xui.RGBColor(0xA4, 0x26, 0x2C)},
		Border:      xui.Style{Fg: xui.RGBColor(0xBF, 0xBF, 0xBF)},
		ToolName:    xui.Style{Fg: xui.RGBColor(0x04, 0x51, 0xA5)},
		SelectionBg: xui.Style{Bg: xui.RGBColor(0x00, 0x78, 0xD4)},
		SelectionFg: xui.Style{Fg: xui.RGBColor(0xFF, 0xFF, 0xFF), Bold: true},
		Keybind:     xui.Style{Fg: xui.RGBColor(0x00, 0x78, 0xD4), Bold: true},
		Command:     xui.Style{Fg: xui.RGBColor(0x00, 0x78, 0xD4)},
		// The picker bar rides the same VS selection blue with white labels;
		// the block highlight is the VS word-find wash.
		PickerSelectionBg:    xui.Style{Bg: xui.RGBColor(0x00, 0x78, 0xD4)},
		PickerSelectionFg:    xui.Style{Fg: xui.RGBColor(0xFF, 0xFF, 0xFF), Bold: true},
		PickerSelectionMuted: xui.Style{Fg: xui.RGBColor(0xD6, 0xE9, 0xF8)},
		BlockHighlight:       xui.Style{Bg: xui.RGBColor(0xFF, 0xF3, 0xC4)},
		// Agent identity mirrors ToolName, the way secondary mirrors it in
		// the opencode palettes.
		Secondary:         xui.Style{Fg: xui.RGBColor(0x04, 0x51, 0xA5)},
		Background:        xui.Style{Bg: xui.RGBColor(0xFF, 0xFF, 0xFF)},
		BackgroundPanel:   xui.Style{Bg: xui.RGBColor(0xEC, 0xEC, 0xEC)},
		BackgroundElement: xui.Style{Bg: xui.RGBColor(0xF5, 0xF5, 0xF5)},
		// VS diff: green / red markers on the classic pale washes.
		DiffAdd:       xui.Style{Fg: xui.RGBColor(0x10, 0x7C, 0x10)},
		DiffRemove:    xui.Style{Fg: xui.RGBColor(0xA4, 0x26, 0x2C)},
		DiffAddedBg:   xui.Style{Bg: xui.RGBColor(0xCC, 0xFF, 0xCC)},
		DiffRemovedBg: xui.Style{Bg: xui.RGBColor(0xFF, 0xCC, 0xCC)},
		Markdown: MarkdownRoles{
			Heading:    xui.Style{Fg: xui.RGBColor(0x00, 0x78, 0xD4), Bold: true},
			Strong:     xui.Style{Fg: xui.RGBColor(0x1F, 0x1F, 0x1F), Bold: true},
			Emph:       xui.Style{Fg: xui.RGBColor(0x1F, 0x1F, 0x1F), Italic: true},
			InlineCode: xui.Style{Fg: xui.RGBColor(0xA3, 0x15, 0x15)},
			Link:       xui.Style{Fg: xui.RGBColor(0x00, 0x78, 0xD4), Underline: true},
			LinkText:   xui.Style{Fg: xui.RGBColor(0x00, 0x78, 0xD4), Underline: true},
			BlockQuote: xui.Style{Fg: xui.RGBColor(0x66, 0x66, 0x66), Italic: true},
			ListItem:   xui.Style{Fg: xui.RGBColor(0x66, 0x66, 0x66)},
			ListEnum:   xui.Style{Fg: xui.RGBColor(0x66, 0x66, 0x66)},
			CodeBlock:  xui.Style{Fg: xui.RGBColor(0x1F, 0x1F, 0x1F)},
		},
		Syntax: SyntaxRoles{ // VS C/C++ defaults
			Comment:     xui.Style{Fg: xui.RGBColor(0x00, 0x80, 0x00)},
			Keyword:     xui.Style{Fg: xui.RGBColor(0x00, 0x00, 0xFF)},
			Function:    xui.Style{Fg: xui.RGBColor(0x79, 0x5E, 0x26)},
			Variable:    xui.Style{Fg: xui.RGBColor(0x1F, 0x1F, 0x1F)},
			String:      xui.Style{Fg: xui.RGBColor(0xA3, 0x15, 0x15)},
			Number:      xui.Style{Fg: xui.RGBColor(0x09, 0x88, 0x5A)},
			Type:        xui.Style{Fg: xui.RGBColor(0x2B, 0x91, 0xAF)},
			Operator:    xui.Style{Fg: xui.RGBColor(0x1F, 0x1F, 0x1F)},
			Punctuation: xui.Style{Fg: xui.RGBColor(0x1F, 0x1F, 0x1F)},
		},
	}
}

// inheritDiffMarkers points the diff marker roles at the palette's own
// success / error text colors; only a palette that ships an explicit pair
// (Light (VS)) skips this inheritance.
func inheritDiffMarkers(th *Theme) {
	th.DiffAdd = th.Success
	th.DiffRemove = th.Destructive
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
// panel paints the terminal default background (visually no panel). The
// picker selection rides each theme's own palette pair, and the block
// highlight is a quiet dark gray that sits on any background.
func legacyChrome(th *Theme) {
	th.Secondary = th.Accent
	th.BackgroundPanel = xui.Style{Bg: xui.DefaultColor()}
	// Legacy themes still own their canvas: without an explicit root the
	// terminal's background shows through and a theme switch repaints text only.
	switch th.Name {
	case "Dark":
		th.Background = xui.Style{Bg: xui.RGBColor(0x1e, 0x1e, 0x1e)}
	case "Darcula":
		th.Background = xui.Style{Bg: xui.RGBColor(0x2b, 0x2b, 0x2b)}
	case "Pink":
		th.Background = xui.Style{Bg: xui.RGBColor(0x23, 0x15, 0x1b)}
	}
	th.BackgroundElement = xui.Style{Bg: xui.DefaultColor()}
	th.PickerSelectionBg = th.SelectionBg
	th.PickerSelectionFg = th.SelectionFg
	th.PickerSelectionMuted = xui.Style{Fg: th.SelectionFg.Fg}
	th.BlockHighlight = xui.Style{Bg: xui.IndexedColor(236)}
	// Legacy palettes carry no backdrop of their own; the 256-color cube's
	// darkest green and red sit on any terminal ground the way 236 does.
	th.DiffAddedBg = xui.Style{Bg: xui.IndexedColor(22)}
	th.DiffRemovedBg = xui.Style{Bg: xui.IndexedColor(52)}
	inheritDiffMarkers(th)
}

// DarkTheme is the fixed RGB dark palette ("Dark").
func DarkTheme() Theme {
	th := Theme{
		Name:        "Dark",
		Foreground:  xui.Style{Fg: xui.DefaultColor()},
		Muted:       xui.Style{Fg: xui.IndexedColor(245)},
		Success:     xui.Style{Fg: xui.RGBColor(0x7d, 0xc3, 0xa0), Bold: true},
		Accent:      xui.Style{Fg: xui.RGBColor(0xc4, 0x8a, 0xd9), Underline: true},
		Warning:     xui.Style{Fg: xui.RGBColor(0xe5, 0xc0, 0x7b)},
		Violet:      xui.Style{Fg: xui.RGBColor(0xc4, 0x8a, 0xd9)},
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
		Name:        "Darcula",
		Foreground:  xui.Style{Fg: xui.RGBColor(0xa9, 0xb7, 0xc6)},
		Muted:       xui.Style{Fg: xui.RGBColor(0x80, 0x80, 0x80), Dim: true},
		Success:     xui.Style{Fg: xui.RGBColor(0x6a, 0x87, 0x59), Bold: true},
		Accent:      xui.Style{Fg: xui.RGBColor(0x58, 0x9d, 0xf6), Underline: true},
		Warning:     xui.Style{Fg: xui.RGBColor(0xcc, 0x78, 0x32)},
		Violet:      xui.Style{Fg: xui.RGBColor(0x9d, 0x7c, 0xd8)},
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
		Name:        "Pink",
		Foreground:  xui.Style{Fg: xui.DefaultColor()},
		Muted:       xui.Style{Fg: xui.RGBColor(0xc8, 0xa0, 0xb4), Dim: true},
		Success:     xui.Style{Fg: xui.RGBColor(0x9e, 0xd4, 0xb8), Bold: true},
		Accent:      xui.Style{Fg: xui.RGBColor(0xff, 0x9e, 0xc8), Underline: true},
		Warning:     xui.Style{Fg: xui.RGBColor(0xff, 0xb8, 0x9a)},
		Violet:      xui.Style{Fg: xui.RGBColor(0xc0, 0x9b, 0xe8)},
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
		Name:        "Terminal",
		Foreground:  xui.Style{Fg: xui.DefaultColor()},
		Muted:       xui.Style{Fg: xui.IndexedColor(8)},
		Success:     xui.Style{Fg: xui.IndexedColor(2), Bold: true},
		Accent:      xui.Style{Fg: xui.IndexedColor(5), Underline: true},
		Warning:     xui.Style{Fg: xui.IndexedColor(3)},
		Violet:      xui.Style{Fg: xui.IndexedColor(5)},
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
	case "light (vs)", "vs-light", "vs light", "opencode-light", "opencode light", "light":
		return VSLightTheme(), true
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
