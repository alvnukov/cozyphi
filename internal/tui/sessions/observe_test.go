package sessions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/editmode"
)

// A surface's account of itself names what it paints with, what it answers
// to, what it would notify through and what it hears — all taken on the
// goroutine that owns the widgets, and all detached.
func TestASurfaceAccountsForWhatItPaintsWithAndWhatItAnswersTo(t *testing.T) {
	e, n := newNotifyTestEditor(t)
	n.facts = diag.NotifierFacts{Known: true, Mode: "unfocused", Sound: "Ping", FocusTrusted: true}

	facts := e.observeSurface()
	require.True(t, facts.Known)
	assert.Equal(t, diag.UIShapeTerminal, facts.Shape)
	assert.Equal(t, "opencode", facts.Theme.Boot, "the palette this surface was built with")
	assert.Equal(t, "opencode", facts.Theme.Live)
	assert.Equal(t, components.ThemeNames(), facts.Theme.Builtin)
	assert.True(t, facts.Keys.Known)
	assert.Equal(t, "unfocused", facts.Notifications.Mode)
	assert.False(t, facts.Voice.Known, "nothing configured speech input for this surface")
	assert.NotEmpty(t, facts.Revision)
}

// The boot palette is what makes "it looked different when I started"
// answerable, so it is kept rather than overwritten by the switch.
func TestSwitchingThePaletteKeepsTheOneTheSurfaceStartedUnder(t *testing.T) {
	e, _ := newNotifyTestEditor(t)
	before := e.observeSurface()

	e.ApplyTheme("Pink")

	after := e.observeSurface()
	assert.Equal(t, "opencode", after.Theme.Boot, "what it started under does not move")
	assert.Equal(t, "Pink", after.Theme.Live)
	assert.NotEqual(t, before.Revision, after.Revision, "two states are visibly two")
}

// The composer's dialect is this session's and the binding table is the
// process's. They are one answer in a session that is working, and reporting
// them apart is what lets one that is not say so.
func TestTheComposersDialectIsReportedBesideTheProcessWideTable(t *testing.T) {
	e, _ := newNotifyTestEditor(t)
	require.NoError(t, e.applyEditingMode(editmode.Readline))

	facts := e.observeSurface()
	assert.Equal(t, "readline", facts.Keys.Editing)
	assert.NotEmpty(t, facts.Keys.Profile)
}

// Wiring a notifier is a lifecycle point, so the surface republishes — and
// asking a notifier what it would do is not asking it to do it.
func TestObservingTheNotifierIsNotNotifyingThroughIt(t *testing.T) {
	e, n := newNotifyTestEditor(t)
	require.Positive(t, n.observed, "wiring the notifier published an account of the surface")

	for range 5 {
		e.observeSurface()
	}

	assert.Zero(t, n.turns, "not one turn ping")
	assert.Empty(t, n.attention, "and nothing asked for attention")
	assert.Empty(t, n.reconfigs, "observing reconfigures nothing")
}

// The fingerprint exists so two snapshots of two states are visibly two. It
// is not a counter: a surface that did not change reports the same one.
func TestTheFingerprintMovesWithTheStateAndNotWithTheAsking(t *testing.T) {
	e, n := newNotifyTestEditor(t)
	first := e.observeSurface().Revision
	assert.Equal(t, first, e.observeSurface().Revision, "asking twice is not a change")

	n.facts = diag.NotifierFacts{Known: true, Mode: "always", Broken: true}
	assert.NotEqual(t, first, e.observeSurface().Revision, "a sender that failed is a change")
}

// A View without a controller has nowhere to publish, and every lifecycle
// point calls this without knowing that. It must be a no-op rather than a
// panic.
func TestPublishingFromASurfaceWithNowhereToPublishIsANoOp(t *testing.T) {
	var absent *View
	assert.NotPanics(t, absent.publishUIStatus)
	assert.NotPanics(t, (&View{}).publishUIStatus)
}
