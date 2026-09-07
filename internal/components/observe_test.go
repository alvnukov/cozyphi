package components

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A palette travels as the name the picker knows it by and as nothing else:
// the colors and attributes are the renderer's business.
func TestAPaletteTravelsAsItsNameAndNothingElse(t *testing.T) {
	facts := ObserveTheme(OpencodeTheme(), PinkTheme())

	require.True(t, facts.Known)
	assert.Equal(t, "opencode", facts.Boot)
	assert.Equal(t, "Pink", facts.Live)
	assert.Equal(t, ThemeNames(), facts.Builtin, "the picker's own list, in its own order")
}

// Every palette this build carries can name itself, or the picker would be
// able to switch to one the diagnostics could not report.
func TestEveryBuiltinPaletteCanNameItself(t *testing.T) {
	for _, name := range ThemeNames() {
		theme, ok := ThemeByName(name)
		require.True(t, ok, "%q is offered by the picker", name)
		assert.Equal(t, name, theme.Name, "the palette answers to the name it is offered under")
	}
}

// A surface nobody switched reports one palette twice rather than looking as
// if something changed.
func TestASurfaceNobodySwitchedReportsOnePaletteTwice(t *testing.T) {
	facts := ObserveTheme(DefaultTheme(), DefaultTheme())
	assert.Equal(t, facts.Boot, facts.Live)
}
