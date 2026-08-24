package components

import (
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

func TestOpencodeLightThemeSelectionReadable(t *testing.T) {
	th := OpencodeLightTheme()
	assert.Equal(t, xui.Style{Bg: xui.RGBColor(0x3b, 0x7d, 0xd8)}, th.SelectionBg)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xff, 0xff, 0xff), Bold: true}, th.SelectionFg)
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

// TestOpencodeLightThemeMarkdownRoles spot-checks the light variant roles
// (opencode.json light defs): amber headings, green inline code, teal links.
func TestOpencodeLightThemeMarkdownRoles(t *testing.T) {
	md := OpencodeLightTheme().Markdown
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xd6, 0x8c, 0x27), Bold: true}, md.Heading)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x3d, 0x9a, 0x57)}, md.InlineCode)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x31, 0x87, 0x95), Underline: true}, md.LinkText)
	sy := OpencodeLightTheme().Syntax
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xd6, 0x8c, 0x27)}, sy.Keyword)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x3d, 0x9a, 0x57)}, sy.String)
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
