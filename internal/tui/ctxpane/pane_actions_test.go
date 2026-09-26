package ctxpane

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/components"
)

// actionsHarness is a pane with the message actions wired to recorders and
// offers that say: every message but the newest reply is a rewind, every
// message is a fork, and the turn is idle unless the test says otherwise.
type actionsHarness struct {
	p                      *Pane
	rewound, forked, asked string
	busy                   bool
	offersRead             int
}

func newActionsHarness() *actionsHarness {
	h := &actionsHarness{}
	view := fixtureView()
	h.p = New(components.DefaultTheme(), func() agent.ContextView { return view }, nil, nil, nil, nil)
	rewind := map[string]struct{}{}
	fork := map[string]struct{}{}
	for i, item := range view.Items {
		if !isMessage(item) {
			continue
		}
		fork[item.EntryID] = struct{}{}
		if i != len(view.Items)-1 {
			rewind[item.EntryID] = struct{}{}
		}
	}
	h.p.SetMessageActions(
		func(id string) { h.rewound = id },
		func(id string) { h.forked = id },
		func(id string) { h.asked = id },
		func() ActionOffers {
			h.offersRead++
			return ActionOffers{Rewind: rewind, Fork: fork, Busy: h.busy}
		},
	)
	return h
}

func selectRow(t *testing.T, p *Pane, row int) {
	t.Helper()
	require.True(t, press(t, p, xui.KeyHome, 0))
	for range row {
		require.True(t, press(t, p, xui.KeyDown, 0))
	}
	require.Equal(t, row, p.cursor.Selected())
}

func menuLabels(t *testing.T, p *Pane) []string {
	t.Helper()
	require.True(t, press(t, p, xui.KeyRune, '.'))
	require.NotNil(t, p.menu)
	labels := make([]string, 0, len(p.menu))
	for _, item := range p.menu {
		labels = append(labels, item.Label)
	}
	require.True(t, press(t, p, xui.KeyEscape, 0))
	return labels
}

func TestPaneRewindOnOfferedPromptClosesAndHandsTheEntryOver(t *testing.T) {
	h := newActionsHarness()
	h.p.Show()
	selectRow(t, h.p, 1)

	require.True(t, press(t, h.p, xui.KeyRune, 'r'))
	assert.Equal(t, "userfix the login bug", h.rewound)
	assert.False(t, h.p.Visible(), "the browser closes before the cut redraws the feed")
	assert.Empty(t, h.forked)
	assert.Empty(t, h.asked)
}

func TestPaneRewindRefusesWhereNoCutIsOffered(t *testing.T) {
	h := newActionsHarness()
	h.p.Show()

	selectRow(t, h.p, 3)
	require.True(t, press(t, h.p, xui.KeyRune, 'r'))
	assert.Equal(t, noticeNotMessage, h.p.notice, "a tool result is no message")

	selectRow(t, h.p, 0)
	require.True(t, press(t, h.p, xui.KeyRune, 'r'))
	assert.Equal(t, noticeNotMessage, h.p.notice, "a summary is no message")

	selectRow(t, h.p, 5)
	require.True(t, press(t, h.p, xui.KeyRune, 'r'))
	assert.Equal(t, noticeRewindTail, h.p.notice, "the newest reply is where the context already ends")

	assert.Empty(t, h.rewound)
	assert.True(t, h.p.Visible(), "a refusal keeps the browser open")
}

func TestPaneRewindNamesTheBoundaryRuleOnAMiddleReply(t *testing.T) {
	h := newActionsHarness()
	h.p.Show()
	// The reply on row 2 is offered by the fixture; take it away to see the
	// footer name the rule rather than the tail.
	h.p.SetMessageActions(
		func(id string) { h.rewound = id }, nil, nil,
		func() ActionOffers { return ActionOffers{} },
	)
	selectRow(t, h.p, 2)
	require.True(t, press(t, h.p, xui.KeyRune, 'r'))
	assert.Equal(t, noticeNotBoundary, h.p.notice)
	assert.Empty(t, h.rewound)
}

func TestPaneMessageActionsRefuseWhileATurnRuns(t *testing.T) {
	h := newActionsHarness()
	h.p.Show()
	h.busy = true
	selectRow(t, h.p, 1)

	for _, key := range []rune{'r', 'f', 'b'} {
		require.True(t, press(t, h.p, xui.KeyRune, key))
		assert.Equal(t, noticeBusy, h.p.notice, "key %q", key)
	}
	assert.Empty(t, h.rewound)
	assert.Empty(t, h.forked)
	assert.Empty(t, h.asked)
	assert.True(t, h.p.Visible())
	assert.NotContains(t, menuLabels(t, h.p), "Rewind before this prompt, drop the tail (r)")
}

func TestPaneForkOnBoundaryClosesAndHandsTheEntryOver(t *testing.T) {
	h := newActionsHarness()
	h.p.Show()

	selectRow(t, h.p, 5)
	require.True(t, press(t, h.p, xui.KeyRune, 'f'))
	assert.Equal(t, "assistantfound it: nil check missing", h.forked,
		"the newest reply is a fork boundary even though it is no rewind")
	assert.False(t, h.p.Visible())

	h = newActionsHarness()
	h.p.Show()
	selectRow(t, h.p, 3)
	require.True(t, press(t, h.p, xui.KeyRune, 'f'))
	assert.Equal(t, noticeNotMessage, h.p.notice)
	assert.Empty(t, h.forked)
}

func TestPaneAsideTakesAnyMessageAndNothingElse(t *testing.T) {
	h := newActionsHarness()
	h.p.Show()

	selectRow(t, h.p, 3)
	require.True(t, press(t, h.p, xui.KeyRune, 'b'))
	assert.Equal(t, noticeNotMessage, h.p.notice)
	selectRow(t, h.p, 0)
	require.True(t, press(t, h.p, xui.KeyRune, 'b'))
	assert.Equal(t, noticeNotMessage, h.p.notice)
	assert.Empty(t, h.asked)

	selectRow(t, h.p, 5)
	require.True(t, press(t, h.p, xui.KeyRune, 'b'))
	assert.Equal(t, "assistantfound it: nil check missing", h.asked,
		"btw needs no boundary, the newest reply will do")
	assert.False(t, h.p.Visible(), "the composer takes over from here")
}

// The menu lists an action only where the key would run it, so a row that
// can only refuse never shows a dead item.
func TestPaneMenuListsMessageActionsOnlyWhereTheyRun(t *testing.T) {
	h := newActionsHarness()
	h.p.Show()

	selectRow(t, h.p, 1)
	labels := menuLabels(t, h.p)
	assert.Contains(t, labels, "Rewind before this prompt, drop the tail (r)")
	assert.Contains(t, labels, "Fork a new tab before this prompt (f)")
	assert.Contains(t, labels, "Ask btw about this message (b)")
	assert.Contains(t, labels, "Trim context up to here, keep the tail (t)")
	assert.Contains(t, labels, "Refresh (R)")

	selectRow(t, h.p, 5)
	labels = menuLabels(t, h.p)
	assert.NotContains(t, labels, "Rewind after this reply, drop the tail (r)")
	assert.Contains(t, labels, "Fork a new tab after this reply (f)")
	assert.Contains(t, labels, "Ask btw about this message (b)")

	selectRow(t, h.p, 3)
	labels = menuLabels(t, h.p)
	for _, label := range labels {
		assert.NotContains(t, label, "(r)")
		assert.NotContains(t, label, "(f)")
		assert.NotContains(t, label, "(b)")
	}

	selectRow(t, h.p, 2)
	require.True(t, press(t, h.p, xui.KeyRune, '.'))
	require.NotNil(t, h.p.menu)
	// Rows: View block, Trim, Rewind; the cursor opens on the first.
	require.True(t, press(t, h.p, xui.KeyDown, 0))
	require.True(t, press(t, h.p, xui.KeyDown, 0))
	require.True(t, press(t, h.p, xui.KeyEnter, 0))
	assert.Equal(t, "assistantlooking at auth.go", h.rewound, "the menu row runs the same action as the key")
	assert.False(t, h.p.Visible())
}

// A refusal is drawn where the footer hints were, in the warning style, and
// the next key clears it.
func TestPaneRefusalReachesTheFooter(t *testing.T) {
	h := newActionsHarness()
	h.p.Show()
	selectRow(t, h.p, 3)
	require.True(t, press(t, h.p, xui.KeyRune, 'r'))
	assert.Contains(t, renderText(t, h.p, 100, 20), noticeNotMessage)
	require.True(t, press(t, h.p, xui.KeyDown, 0))
	assert.Empty(t, h.p.notice)
}

// Refresh moved to Shift+r to give r to the rewind. The uppercase rune must
// re-read the snapshot however the terminal reports the shift: as a bare
// uppercase rune, with the shift bit set, and from the Russian layout, where
// Shift+к lands on the same physical key.
func TestPaneShiftRRefreshesAndLowercaseRDoesNot(t *testing.T) {
	h := newActionsHarness()
	h.p.Show()
	reads := h.offersRead

	for _, ev := range []xui.KeyEvent{
		{Press: true, Code: xui.KeyRune, Rune: 'R'},
		{Press: true, Code: xui.KeyRune, Rune: 'R', Mods: xui.ModShift},
		{Press: true, Code: xui.KeyRune, Rune: 'К', Mods: xui.ModShift},
	} {
		before := h.offersRead
		require.True(t, h.p.HandleEvent(&components.EventContext{}, ev))
		assert.Equal(t, before+1, h.offersRead, "%+v re-reads the snapshot and the offers", ev)
		assert.True(t, h.p.Visible())
		assert.Empty(t, h.rewound)
	}
	assert.Equal(t, reads+3, h.offersRead)

	selectRow(t, h.p, 1)
	reads = h.offersRead
	renderText(t, h.p, 100, 20)
	assert.Equal(t, reads, h.offersRead, "a Draw never asks the engine")
	require.True(t, press(t, h.p, xui.KeyRune, 'r'))
	assert.Equal(t, "userfix the login bug", h.rewound, "lowercase r is the rewind now")
	assert.Equal(t, reads+1, h.offersRead, "an attempt re-reads the offers once")
}

func TestPaneUnwiredMessageActionsStayQuiet(t *testing.T) {
	p, _, _, _ := newTestPane()
	p.Show()
	selectRow(t, p, 1)
	for _, key := range []rune{'r', 'f', 'b'} {
		require.True(t, press(t, p, xui.KeyRune, key))
		assert.Empty(t, p.notice)
		assert.True(t, p.Visible())
	}
	labels := menuLabels(t, p)
	assert.NotContains(t, labels, "Ask btw about this message (b)")
}
