package keys

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/editmode"
)

// The configuration half reports the vocabulary a keybinds section may name
// and which of its commands were named — and nothing at all about the chords
// they were given, because a chord is what the help screen is for.
func TestTheConfiguredKeyboardTravelsAsCommandNamesAndNeverAsChords(t *testing.T) {
	facts := ObserveConfig(map[string]string{"palette": "F9", "help": "F2"})

	require.True(t, facts.Known)
	assert.Equal(t, []string{"help", "palette"}, facts.Overrides, "sorted, so two snapshots compare")
	assert.Len(t, facts.Commands, len(commands))
	assert.Equal(t, string(commands[0]), facts.Commands[0], "the vocabulary keeps dispatch order")
	assertNoChords(t, append(facts.Commands, facts.Overrides...))
}

// A configuration that rebinds nothing is not a configuration nobody read:
// the vocabulary is still the answer, and the override list is empty rather
// than missing.
func TestAKeyboardNobodyRebound(t *testing.T) {
	facts := ObserveConfig(nil)
	require.True(t, facts.Known)
	assert.Empty(t, facts.Overrides)
	assert.NotEmpty(t, facts.Commands)
}

// The runtime half is the compiled table's own account: the dialect it was
// built for, what it took an override for, and what it actually spells
// differently. With nothing rebound the last two are empty and the first is
// the house dialect.
func TestATableNobodyTouchedReportsTheHouseDialectAndNoDivergence(t *testing.T) {
	restoreDefaults(t)
	require.NoError(t, Rebind(nil))

	facts := Observe()
	require.True(t, facts.Known)
	assert.Equal(t, editmode.Standard.String(), facts.Profile)
	assert.Empty(t, facts.Accepted)
	assert.Empty(t, facts.Diverged)
}

// A rebinding shows up in both lists: it was taken, and it changed something.
func TestARebindingIsBothAcceptedAndDiverged(t *testing.T) {
	restoreDefaults(t)
	require.NoError(t, Rebind(map[string]string{"palette": "F9"}))

	facts := Observe()
	assert.Equal(t, []string{"palette"}, facts.Accepted)
	assert.Equal(t, []string{"palette"}, facts.Diverged)
	assertNoChords(t, append(facts.Accepted, facts.Diverged...))
}

// The two lists are reported separately because they come apart, and this is
// how: a rebinding that names the chord the dialect would have given anyway
// was taken and changed nothing. Reporting only one list would make a
// configuration that does nothing look like one that does.
func TestARebindingToTheChordItAlreadyHadIsTakenAndChangesNothing(t *testing.T) {
	restoreDefaults(t)
	require.NoError(t, Rebind(map[string]string{"help": "F1"}))

	facts := Observe()
	assert.Equal(t, []string{"help"}, facts.Accepted, "the configuration named it")
	assert.Empty(t, facts.Diverged, "and F1 is what help answers to anyway")
}

// Switching dialects moves the defaults under a rebinding that never changed,
// so the same override diverges in one dialect and not in the next. This is
// the case a person hits after /keymap and cannot explain.
func TestSwitchingDialectsMovesTheDefaultsUnderAnUnchangedRebinding(t *testing.T) {
	restoreDefaults(t)
	require.NoError(t, Rebind(map[string]string{"palette": "F2"}))
	assert.Equal(t, []string{"palette"}, Observe().Diverged, "standard opens the palette with Ctrl+K")

	require.NoError(t, SetProfile(editmode.Readline))
	facts := Observe()
	assert.Equal(t, editmode.Readline.String(), facts.Profile)
	assert.Equal(t, []string{"palette"}, facts.Accepted, "the override is still the one somebody wrote")
	assert.Empty(t, facts.Diverged, "and readline opens the palette with F2 on its own")
}

// Observing must not be the thing that changes what is observed: the table,
// the dialect and every label are exactly what they were.
func TestObservingTheKeyboardCompilesNothingAndRebindsNothing(t *testing.T) {
	restoreDefaults(t)
	require.NoError(t, Rebind(map[string]string{"palette": "F9"}))
	before := Label(CmdPalette)
	profile := activeProfile

	for range 3 {
		Observe()
		ObserveConfig(map[string]string{"palette": "F9"})
	}

	assert.Equal(t, before, Label(CmdPalette))
	assert.Equal(t, profile, activeProfile)
}

// assertNoChords is the exclusion this projection exists to keep: not one of
// these strings may be a key.
func assertNoChords(t *testing.T, values []string) {
	t.Helper()
	for _, value := range values {
		assert.NotContains(t, value, "+", value)
		assert.NotContains(t, strings.ToLower(value), "ctrl", value)
		assert.False(t, strings.HasPrefix(value, "F") && len(value) <= 3, "%q reads as a function key", value)
	}
}
