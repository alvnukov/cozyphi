package transcript

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/session"
)

// The answer to a side question streams into its row the way a reply does:
// each update is a patch of the bottom row, never a rebuild of the feed.
func TestAnAsideStreamsThroughTheTailPatch(t *testing.T) {
	pane := NewTranscriptPane(components.DefaultTheme(), nil, "test")
	pane.ApplySession(session.UserAppend{ID: "u1", Text: "hello"})
	pane.ApplySession(session.AsideUpdate{ID: "x1", Question: "side?", State: session.StateStreaming})
	pane.Sync()
	require.Equal(t, []string{"u1", "x1"}, pane.listIDs)
	row, ok := pane.list.Entries[1].(*block.AsideBlock)
	require.True(t, ok, "widget %T", pane.list.Entries[1])

	for _, answer := range []string{"one", "one two", "one two three"} {
		pane.ApplySession(
			session.AsideUpdate{ID: "x1", Question: "side?", Answer: answer, State: session.StateStreaming},
		)
		assert.Equal(t, projectionSyncTail, pane.syncMode, "an update of the bottom aside is a tail patch")
		pane.Sync()
		assert.Same(t, row, pane.list.Entries[1])
		assert.Equal(t, answer, row.Answer)
	}
	pane.ApplySession(
		session.AsideUpdate{ID: "x1", Question: "side?", Answer: "one two three", State: session.StateComplete},
	)
	assert.Equal(t, projectionSyncTail, pane.syncMode)
	pane.Sync()
	assert.Equal(t, session.StateComplete, row.State)

	// A row added below the aside ends the shortcut: the aside is no longer
	// the tail, so its next update takes the full pass.
	pane.ApplySession(session.UserAppend{ID: "u2", Text: "next"})
	pane.Sync()
	pane.ApplySession(session.AsideUpdate{ID: "x1", Question: "side?", Answer: "late", State: session.StateComplete})
	assert.Equal(t, projectionSyncFull, pane.syncMode)
}

// A theme switch repaints the aside rows already in the feed: the bar and the
// btw label take the new theme's color on the next frame.
func TestAThemeSwitchRepaintsAnAside(t *testing.T) {
	dark, light := components.DarkTheme(), components.VSLightTheme()
	pane := NewTranscriptPane(dark, nil, "test")
	pane.ApplySession(session.AsideUpdate{ID: "x1", Question: "side?", Answer: "yes", State: session.StateComplete})
	pane.Sync()
	row, ok := pane.list.Entries[0].(*block.AsideBlock)
	require.True(t, ok, "widget %T", pane.list.Entries[0])

	pane.SetTheme(light)
	s := row.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 10}})
	for y := range s.Size.Height {
		assert.Equal(t, light.Aside.Fg, s.Buffer[y*s.Size.Width].Style.Fg, "gutter of row %d", y)
	}
}
