package components

import (
	"math"
	"reflect"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultThemeIsOpencode(t *testing.T) {
	assert.Equal(t, OpencodeTheme(), DefaultTheme(), "startup look must be the opencode palette")
}

func TestThemeNamesListOpencodeFirst(t *testing.T) {
	names := ThemeNames()
	require.NotEmpty(t, names)
	assert.Equal(t, "opencode", names[0], "opencode is the house theme — picker leads with it")
}

func TestThemeByNameResolvesOpencode(t *testing.T) {
	for _, name := range []string{"opencode", "OpenCode", "opencode-light", "opencode light"} {
		th, ok := ThemeByName(name)
		if assert.True(t, ok, "theme %q must resolve", name) {
			assertAllSlotsSet(t, th, name)
		}
	}
	_, ok := ThemeByName("no-such-theme")
	assert.False(t, ok)
}

// TestThemeByNameResolvesVSLight: the light palette answers to its display
// name and every historical alias, so saved configs keep working.
func TestThemeByNameResolvesVSLight(t *testing.T) {
	for _, name := range []string{"Light (VS)", "light (vs)", "vs-light", "vs light", "opencode-light", "light"} {
		th, ok := ThemeByName(name)
		if assert.True(t, ok, "theme %q must resolve", name) {
			assertAllSlotsSet(t, th, name)
			assert.Equal(t, "Light (VS)", th.Name, "alias %q must resolve to the display name", name)
		}
	}
}

// TestOpencodeThemePortedPalette pins the port against the upstream asset
// (sst/opencode packages/tui/src/theme/assets/opencode.json, dark variant):
// orange primary, blue secondary, red error, near-black selection foreground.
func TestOpencodeThemePortedPalette(t *testing.T) {
	th := OpencodeTheme()
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xee, 0xee, 0xee)}, th.Foreground)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xfa, 0xb2, 0x83), Underline: true}, th.Accent)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x5c, 0x9c, 0xf5)}, th.ToolName)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xe0, 0x6c, 0x75)}, th.Destructive)
	assert.Equal(t, xui.Style{Bg: xui.RGBColor(0xfa, 0xb2, 0x83)}, th.SelectionBg)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x0a, 0x0a, 0x0a), Bold: true}, th.SelectionFg)
}

func TestVSLightThemeSelectionReadable(t *testing.T) {
	th := VSLightTheme()
	assert.Equal(t, xui.Style{Bg: xui.RGBColor(0x00, 0x78, 0xD4)}, th.SelectionBg)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xFF, 0xFF, 0xFF), Bold: true}, th.SelectionFg)
}

// TestOpencodeThemeMarkdownRoles pins the prose roles against the upstream
// asset (opencode.json "theme.markdown*"): purple bold headings, orange
// strong, yellow emphasis and quotes, green inline code, cyan link labels,
// peach bullets, cyan enumerations, plain code-block text.
func TestOpencodeThemeMarkdownRoles(t *testing.T) {
	md := OpencodeTheme().Markdown
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x9d, 0x7c, 0xd8), Bold: true}, md.Heading)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xf5, 0xa7, 0x42), Bold: true}, md.Strong)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xe5, 0xc0, 0x7b), Italic: true}, md.Emph)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x7f, 0xd8, 0x8f)}, md.InlineCode)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xfa, 0xb2, 0x83), Underline: true}, md.Link)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x56, 0xb6, 0xc2), Underline: true}, md.LinkText)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xe5, 0xc0, 0x7b), Italic: true}, md.BlockQuote)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xfa, 0xb2, 0x83)}, md.ListItem)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x56, 0xb6, 0xc2)}, md.ListEnum)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xee, 0xee, 0xee)}, md.CodeBlock)
}

// TestOpencodeThemeSyntaxRoles pins code highlighting to opencode.json
// "theme.syntax*": muted comments, purple keywords, peach functions, red
// variables, green strings, orange numbers, yellow types, cyan operators.
func TestOpencodeThemeSyntaxRoles(t *testing.T) {
	sy := OpencodeTheme().Syntax
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x80, 0x80, 0x80)}, sy.Comment)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x9d, 0x7c, 0xd8)}, sy.Keyword)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xfa, 0xb2, 0x83)}, sy.Function)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xe0, 0x6c, 0x75)}, sy.Variable)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x7f, 0xd8, 0x8f)}, sy.String)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xf5, 0xa7, 0x42)}, sy.Number)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xe5, 0xc0, 0x7b)}, sy.Type)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x56, 0xb6, 0xc2)}, sy.Operator)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xee, 0xee, 0xee)}, sy.Punctuation)
}

// TestVSLightThemeMarkdownRoles spot-checks the VS light roles: blue bold
// headings, maroon inline code, blue links, VS C/C++ keyword and string.
func TestVSLightThemeMarkdownRoles(t *testing.T) {
	md := VSLightTheme().Markdown
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x00, 0x78, 0xD4), Bold: true}, md.Heading)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xA3, 0x15, 0x15)}, md.InlineCode)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x00, 0x78, 0xD4), Underline: true}, md.LinkText)
	sy := VSLightTheme().Syntax
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x00, 0x00, 0xFF)}, sy.Keyword)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xA3, 0x15, 0x15)}, sy.String)
}

// TestLegacyThemesKeepProseLook: Dark/Darcula/Pink/Terminal keep their
// pre-role look — inline code and plain code stay warning, markers and quotes
// stay muted, link labels stay accent. Only the opencode themes carry the
// upstream markdown palette.
func TestLegacyThemesKeepProseLook(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   Theme
	}{
		{"Dark", DarkTheme()},
		{"Darcula", DarculaTheme()},
		{"Pink", PinkTheme()},
		{"Terminal", TerminalTheme()},
	} {
		th := tc.th
		assert.Equal(t, th.Warning.Fg, th.Markdown.InlineCode.Fg, "%s InlineCode", tc.name)
		assert.Equal(t, th.Warning.Fg, th.Markdown.CodeBlock.Fg, "%s CodeBlock", tc.name)
		assert.Equal(t, th.Muted.Fg, th.Markdown.ListItem.Fg, "%s ListItem", tc.name)
		assert.Equal(t, th.Muted.Fg, th.Markdown.BlockQuote.Fg, "%s BlockQuote", tc.name)
		assert.Equal(t, th.Accent.Fg, th.Markdown.LinkText.Fg, "%s LinkText", tc.name)
		assert.Equal(t, th.ToolName.Fg, th.Syntax.Keyword.Fg, "%s Syntax.Keyword", tc.name)
	}
}

// TestOpencodeThemeChromeRoles pins the agent-identity and panel-background
// slots against opencode.json: theme.secondary and theme.backgroundPanel.
func TestOpencodeThemeChromeRoles(t *testing.T) {
	dark := OpencodeTheme()
	assert.Equal(t, xui.RGBColor(0x5c, 0x9c, 0xf5), dark.Secondary.Fg, "dark secondary")
	assert.Equal(t, xui.RGBColor(0x14, 0x14, 0x14), dark.BackgroundPanel.Bg, "dark backgroundPanel")
	assert.Equal(t, xui.RGBColor(0x1e, 0x1e, 0x1e), dark.BackgroundElement.Bg, "dark backgroundElement")

	// The light row pins Light (VS): #0451A5 agent identity on the VS paper
	// grays (#ECECEC panel, #F5F5F5 element).
	light := VSLightTheme()
	assert.Equal(t, xui.RGBColor(0x04, 0x51, 0xA5), light.Secondary.Fg, "light secondary")
	assert.Equal(t, xui.RGBColor(0xEC, 0xEC, 0xEC), light.BackgroundPanel.Bg, "light backgroundPanel")
	assert.Equal(t, xui.RGBColor(0xF5, 0xF5, 0xF5), light.BackgroundElement.Bg, "light backgroundElement")
}

// TestLegacyThemesKeepChromeDefaults: legacy themes have no agent palette,
// so the identity color stays Accent. Only Terminal still paints its panels
// on the terminal's own ground; the RGB legacies own theirs.
func TestLegacyThemesKeepChromeDefaults(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   Theme
	}{
		{"Dark", DarkTheme()},
		{"Darcula", DarculaTheme()},
		{"Pink", PinkTheme()},
		{"Terminal", TerminalTheme()},
	} {
		assert.Equal(t, tc.th.Accent, tc.th.Secondary, "%s Secondary", tc.name)
	}
	terminal := TerminalTheme()
	assert.Equal(t, xui.DefaultColor(), terminal.BackgroundPanel.Bg, "Terminal BackgroundPanel")
	assert.Equal(t, xui.DefaultColor(), terminal.BackgroundElement.Bg, "Terminal BackgroundElement")
}

// TestBuiltinThemesOwnEverySurface: a theme is isolated from the terminal
// profile only when every surface it paints (text, canvas, panels, editor
// element, block highlight, diff washes) carries a color of its own. A
// default anywhere is a hole the terminal shows through. Terminal is the one
// palette that follows the terminal by design and is not held to this.
func TestBuiltinThemesOwnEverySurface(t *testing.T) {
	for _, name := range ThemeNames() {
		if name == "Terminal" {
			continue
		}
		th, ok := ThemeByName(name)
		require.True(t, ok, name)
		for role, c := range map[string]xui.Color{
			"Foreground":        th.Foreground.Fg,
			"Muted":             th.Muted.Fg,
			"Border":            th.Border.Fg,
			"Background":        th.Background.Bg,
			"BackgroundPanel":   th.BackgroundPanel.Bg,
			"BackgroundElement": th.BackgroundElement.Bg,
			"BlockHighlight":    th.BlockHighlight.Bg,
			"DiffAddedBg":       th.DiffAddedBg.Bg,
			"DiffRemovedBg":     th.DiffRemovedBg.Bg,
			"SelectionBg":       th.SelectionBg.Bg,
			"PickerSelectionBg": th.PickerSelectionBg.Bg,
		} {
			assert.Equal(t, xui.ColorRGB, c.Kind, "%s: %s must be an RGB color of the theme's own", name, role)
		}
	}
}

func TestThemeVioletSlotSet(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   Theme
		want xui.Style
	}{
		{"opencode", OpencodeTheme(), xui.Style{Fg: xui.RGBColor(0x9d, 0x7c, 0xd8)}},
		{"Light (VS)", VSLightTheme(), xui.Style{Fg: xui.RGBColor(0x68, 0x21, 0x7A)}},
		{"Dark", DarkTheme(), xui.Style{Fg: xui.RGBColor(0xc4, 0x8a, 0xd9)}},
		{"Darcula", DarculaTheme(), xui.Style{Fg: xui.RGBColor(0x9d, 0x7c, 0xd8)}},
		{"Pink", PinkTheme(), xui.Style{Fg: xui.RGBColor(0xc0, 0x9b, 0xe8)}},
		{"Terminal", TerminalTheme(), xui.Style{Fg: xui.IndexedColor(5)}},
	} {
		assert.Equal(t, tc.want, tc.th.Violet, "%s Violet", tc.name)
	}
}

// TestOpencodeThemePickerAndHighlightRoles pins the picker's soft blue bar —
// deliberately distinct from the palette's primary-yellow selection — and the
// dark olive block-copy tint.
func TestOpencodeThemePickerAndHighlightRoles(t *testing.T) {
	th := OpencodeTheme()
	assert.Equal(t, xui.Style{Bg: xui.RGBColor(0x3a, 0x5a, 0x7a)}, th.PickerSelectionBg)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xe0, 0xf0, 0xff), Bold: true}, th.PickerSelectionFg)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xb0, 0xc8, 0xe0)}, th.PickerSelectionMuted)
	assert.Equal(t, xui.Style{Bg: xui.RGBColor(0x2a, 0x2e, 0x24)}, th.BlockHighlight)
}

// TestLegacyThemesDerivePickerSelection: legacy themes ride their own
// selection pair for the picker bar, so theme switches leave no widget
// stale. Terminal keeps the quiet 256-cube gray for the block highlight that
// sits on any terminal ground; the RGB legacies own theirs.
func TestLegacyThemesDerivePickerSelection(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   Theme
	}{
		{"Dark", DarkTheme()},
		{"Darcula", DarculaTheme()},
		{"Pink", PinkTheme()},
		{"Terminal", TerminalTheme()},
	} {
		assert.Equal(t, tc.th.SelectionBg, tc.th.PickerSelectionBg, "%s PickerSelectionBg", tc.name)
		assert.Equal(t, tc.th.SelectionFg.Fg, tc.th.PickerSelectionFg.Fg, "%s PickerSelectionFg", tc.name)
	}
	assert.Equal(t, xui.IndexedColor(236), TerminalTheme().BlockHighlight.Bg, "Terminal BlockHighlight")
}

// assertAllSlotsSet pins the contract every named theme must meet: each Theme
// slot (including the Markdown and Syntax role groups) carries an explicit
// value, so a slot added later without palette values fails here instead of
// silently rendering terminal-default.
func assertAllSlotsSet(t *testing.T, th Theme, name string) {
	t.Helper()
	assertStyleSlotsSet(t, reflect.ValueOf(th), name)
}

func assertStyleSlotsSet(t *testing.T, v reflect.Value, path string) {
	t.Helper()
	styleType := reflect.TypeFor[xui.Style]()
	typ := v.Type()
	for i := 0; i < typ.NumField(); i++ {
		field, fv := typ.Field(i), v.Field(i)
		switch {
		case field.Name == "Name":
			// The one slot that is not a style: a palette has to be able to
			// name itself, or the picker could switch to one the diagnostics
			// could not report.
			assert.NotEmpty(t, fv.String(), "%s.%s: palette left unnamed", path, field.Name)
		case fv.Type() == styleType:
			style := fv.Interface().(xui.Style)
			assert.NotEqual(t, xui.Style{}, style, "%s.%s: slot left unset", path, field.Name)
		case fv.Kind() == reflect.Struct:
			assertStyleSlotsSet(t, fv, path+"."+field.Name)
		default:
			t.Fatalf("%s.%s: unexpected field type", path, field.Name)
		}
	}
}

// Every themed palette must own its canvas: the root fill in View.Draw and
// the pane fills take theme.Background, so a palette that leaves it at the
// terminal default makes /theme repaint text only. Terminal is the
// deliberate exception: it follows the terminal's own background.
func TestBuiltinThemesOwnTheirBackground(t *testing.T) {
	terminal := TerminalTheme().Background.Bg
	for _, th := range []Theme{
		OpencodeTheme(), VSLightTheme(), DarkTheme(), DarculaTheme(), PinkTheme(),
	} {
		assert.NotEqual(t, terminal, th.Background.Bg,
			"%s must carry an explicit app background", th.Name)
	}
}

// TestVSLightThemeContrast: every role Light (VS) paints on paper must clear
// WCAG AA (4.5:1) against its real backdrop. The two authentic VS C/C++
// values that sit just under AA (type, number) are pinned as accepted
// exceptions so a further dip fails loudly.
func TestVSLightThemeContrast(t *testing.T) {
	th := VSLightTheme()
	for _, p := range []struct {
		name string
		fg   xui.Color
		bg   xui.Color
		min  float64
	}{
		{"foreground on canvas", th.Foreground.Fg, th.Background.Bg, 4.5},
		{"muted on canvas", th.Muted.Fg, th.Background.Bg, 4.5},
		{"success on canvas", th.Success.Fg, th.Background.Bg, 4.5},
		{"warning on canvas", th.Warning.Fg, th.Background.Bg, 4.5},
		{"destructive on canvas", th.Destructive.Fg, th.Background.Bg, 4.5},
		{"tool name on panel", th.ToolName.Fg, th.BackgroundPanel.Bg, 4.5},
		{"diff add marker on wash", th.DiffAdd.Fg, th.DiffAddedBg.Bg, 4.5},
		{"diff remove marker on wash", th.DiffRemove.Fg, th.DiffRemovedBg.Bg, 4.5},
		{"syntax type on canvas (VS value)", th.Syntax.Type.Fg, th.Background.Bg, 3.6},
		{"syntax number on canvas (VS value)", th.Syntax.Number.Fg, th.Background.Bg, 4.4},
	} {
		if got := contrastRatio(p.fg, p.bg); got < p.min {
			t.Errorf("%s: contrast %.2f below %.2f", p.name, got, p.min)
		}
	}
}

// contrastRatio is the WCAG 2.x contrast of two colors; indexed and default
// colors cannot be measured and read as white, which only ever loosens the
// check for themes this test does not cover.
func contrastRatio(a, b xui.Color) float64 {
	la, lb := relLuminance(a), relLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func relLuminance(c xui.Color) float64 {
	if c.Kind != xui.ColorRGB {
		return 1
	}
	channel := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(c.R) + 0.7152*channel(c.G) + 0.0722*channel(c.B)
}
