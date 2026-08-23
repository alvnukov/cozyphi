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

// assertAllSlotsSet pins the contract every named theme must meet: each Theme
// slot carries an explicit value, so a slot added later without palette values
// fails here instead of silently rendering terminal-default.
func assertAllSlotsSet(t *testing.T, th Theme, name string) {
	t.Helper()
	v := reflect.ValueOf(th)
	typ := v.Type()
	for i := 0; i < typ.NumField(); i++ {
		style, ok := v.Field(i).Interface().(xui.Style)
		require.True(t, ok, "%s.%s: unexpected field type", name, typ.Field(i).Name)
		assert.NotEqual(t, xui.Style{}, style, "%s.%s: slot left unset", name, typ.Field(i).Name)
	}
}
