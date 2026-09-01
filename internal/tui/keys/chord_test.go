package keys

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseChordCanonicalizesTheSpelling(t *testing.T) {
	for spelling, want := range map[string]string{
		"F1":           "F1",
		"f2":           "F2",
		"Ctrl+,":       "Ctrl+,",
		"ctrl+shift+c": "Ctrl+Shift+C",
		"alt+p":        "Alt+P",
		"Super+C":      "Cmd+C",
		"cmd+c":        "Cmd+C",
		"Shift+Ctrl+X": "Ctrl+Shift+X",
		"Ctrl++":       "Ctrl++",
	} {
		c, err := ParseChord(spelling)
		require.NoError(t, err, spelling)
		assert.Equal(t, want, c.String(), "spelling %q", spelling)
	}

	padded, err := ParseChord(" Ctrl + K ")
	require.NoError(t, err, "stray spaces are the user's, not an error")
	assert.Equal(t, "Ctrl+K", padded.String())
}

func TestParseChordRejectsWhatCannotBeAChord(t *testing.T) {
	for spelling, why := range map[string]string{
		"":             "empty",
		"p":            "a bare rune is typing",
		"Shift+P":      "shift alone is typing",
		"Meta+P":       "unknown modifier",
		"Ctrl+F badly": "unknown key",
		"Ctrl+Enter":   "named keys stop at F1-F12",
	} {
		_, err := ParseChord(spelling)
		assert.Error(t, err, "%q must be rejected: %s", spelling, why)
	}
}

// TestChordMatchIsExact pins the matching contract: modifiers compare
// exactly (Ctrl+P and Ctrl+Shift+P are different chords), runes fold case,
// releases never match.
func TestChordMatchIsExact(t *testing.T) {
	ctrlP, err := ParseChord("Ctrl+P")
	require.NoError(t, err)
	press := func(r rune, mods xui.Modifiers) xui.KeyEvent {
		return xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: r, Mods: mods}
	}

	assert.True(t, ctrlP.Match(press('p', xui.ModCtrl)))
	assert.True(t, ctrlP.Match(press('P', xui.ModCtrl)), "case folds")
	assert.False(t, ctrlP.Match(press('p', xui.ModCtrl|xui.ModShift)), "extra modifier is a different chord")
	assert.False(t, ctrlP.Match(press('p', xui.ModAlt)), "wrong modifier")
	assert.False(t, ctrlP.Match(xui.KeyEvent{Press: false, Code: xui.KeyRune, Rune: 'p', Mods: xui.ModCtrl}),
		"a release is not a press")

	f1, err := ParseChord("F1")
	require.NoError(t, err)
	assert.True(t, f1.Match(xui.KeyEvent{Press: true, Code: xui.KeyF1}))
	assert.False(t, f1.Match(xui.KeyEvent{Press: true, Code: xui.KeyF1, Mods: xui.ModCtrl}),
		"bare F1 means bare")

	cmdC, err := ParseChord("Cmd+C")
	require.NoError(t, err)
	assert.True(t, cmdC.Match(press('c', xui.ModSuper)))
	assert.False(t, cmdC.Match(press('c', xui.ModSuper|xui.ModCtrl)))
}
