package notify_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/notify"
)

// What the configuration asked for survives a process with no notifier at
// all, so it is projected from the decoded values rather than from a live one.
func TestTheConfiguredNotificationsAreReportedWithoutANotifier(t *testing.T) {
	facts := notify.ObserveConfig(notify.ModeUnfocused, "Ping")
	assert.Equal(t, "unfocused", facts.Mode)
	assert.Equal(t, "Ping", facts.Sound)
	require.True(t, facts.Known)

	silent := notify.ObserveConfig(notify.ModeAlways, "")
	assert.Empty(t, silent.Sound, "silence is a setting and travels as one")
}

// The live notifier reports what it would do and what it has already found
// out: the mode it is in, the sound it would ask for, and whether the
// terminal's focus reports may be believed yet.
func TestALiveNotifierReportsWhatItWouldDo(t *testing.T) {
	n := notify.New(notify.ModeUnfocused, notify.WithSound("Ping"),
		notify.WithSender(func(context.Context, string, string) error { return nil }))

	facts := n.Observe()
	require.True(t, facts.Known)
	assert.Equal(t, "unfocused", facts.Mode)
	assert.Equal(t, "Ping", facts.Sound)
	assert.False(t, facts.Broken)
	assert.False(t, facts.FocusTrusted, "no terminal has reported losing focus yet")
	assert.True(t, facts.Focused, "and until one does, a session is assumed to be looked at")

	n.SetFocused(false)
	facts = n.Observe()
	assert.True(t, facts.FocusTrusted, "a report of lost focus is what makes the next one believable")
	assert.False(t, facts.Focused)
}

// A sender that failed switched notifications off for the rest of the
// session, and the mode still says otherwise. That gap is the whole reason
// the notifier is asked rather than the configuration.
func TestASenderThatFailedIsVisibleWhileTheModeStillSaysOtherwise(t *testing.T) {
	failed := make(chan struct{}, 1)
	n := notify.New(notify.ModeAlways,
		notify.WithSender(func(context.Context, string, string) error { return errors.New("sender failed") }))
	n.SetOnFailure(func(error) { failed <- struct{}{} })

	require.False(t, n.Observe().Broken)
	n.TurnEnded()
	<-failed

	facts := n.Observe()
	assert.True(t, facts.Broken)
	assert.Equal(t, "always", facts.Mode, "nothing changed the mode; the sender simply stopped working")
}

// A reconfiguration is what the settings pane applies, and the projection
// follows it without being told twice.
func TestObservingFollowsAReconfiguration(t *testing.T) {
	n := notify.New(notify.ModeAlways, notify.WithSound("Ping"),
		notify.WithSender(func(context.Context, string, string) error { return nil }))
	n.Reconfigure(notify.ModeOff, "")

	facts := n.Observe()
	assert.Equal(t, "off", facts.Mode)
	assert.Empty(t, facts.Sound)
}

// A notifier nobody attached is absence, not a mode nobody set: a nil one is
// how a headless run and a surface still being built both look, and neither
// is "notifications are off".
func TestANotifierNobodyAttachedIsAbsenceAndNotAModeNobodySet(t *testing.T) {
	var n *notify.Notifier
	facts := n.Observe()
	assert.False(t, facts.Known)
	assert.Empty(t, facts.Mode)
}

// Asking must never be what notifies: no title, no body and no error text
// travels, and the sender is not called even once.
func TestObservingSendsNothingAndCarriesNoNotificationText(t *testing.T) {
	var sends int
	n := notify.New(notify.ModeAlways, notify.WithSound("Ping"),
		notify.WithSender(func(context.Context, string, string) error { sends++; return nil }))
	n.SetOrigin("cozyphi")

	for range 5 {
		facts := n.Observe()
		assert.NotContains(t, facts.Mode, "cozyphi")
		assert.NotContains(t, facts.Sound, "cozyphi")
	}
	assert.Zero(t, sends, "a question about notifications is not one")
}
