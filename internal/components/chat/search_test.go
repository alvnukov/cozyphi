package chat

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/history"
)

// runeKey builds a printable typing press for ChatInput.Handle.
func runeKey(r rune) xui.KeyEvent {
	return xui.KeyEvent{Code: xui.KeyRune, Rune: r, Press: true}
}

// newSearchInput builds a ChatInput over a store seeded with entries, plus
// the submitted-text sink the Enter tests read.
func newSearchInput(t *testing.T, entries ...string) (*ChatInput, *[]string) {
	t.Helper()
	h := history.Open("")
	for _, e := range entries {
		h.Append(e)
	}
	var submitted []string
	c := &ChatInput{MinBodyRows: 3, History: h}
	c.OnSubmit = func(text string) { submitted = append(submitted, text) }
	return c, &submitted
}

// typeQuery feeds the query one rune at a time, the way the keyboard does.
func typeQuery(c *ChatInput, q string) {
	for _, r := range q {
		c.Handle(&components.EventContext{}, runeKey(r))
	}
}

// acceptByEscape ends the mode keeping the match, the read-out the tests use
// to see which match the mode sits on.
func acceptByEscape(c *ChatInput) {
	c.Handle(&components.EventContext{}, key(xui.KeyEscape))
}

// TestBeginSearchSavesDraftAndAbortRestoresIt: entry captures the draft, and
// Ctrl+G's abort hands it back exactly — match discarded, nothing submitted.
func TestBeginSearchSavesDraftAndAbortRestoresIt(t *testing.T) {
	c, submitted := newSearchInput(t, "a match")
	c.Value = "my draft"
	c.Cursor = len("my ")

	require.True(t, c.BeginSearch())
	require.True(t, c.SearchActive())
	typeQuery(c, "match")

	c.SearchAbort()

	require.False(t, c.SearchActive())
	require.Equal(t, "my draft", c.Value)
	require.Equal(t, len("my "), c.Cursor)
	require.Empty(t, *submitted)
}

// TestBeginSearchWithoutHistoryRefuses: no store, no mode — the chord must
// stay unconsumed, so BeginSearch reports false.
func TestBeginSearchWithoutHistoryRefuses(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3}
	require.False(t, c.BeginSearch())
	require.False(t, c.SearchActive())
}

// TestTypingEditsQueryNeverTheBuffer: while the mode is on, typing and
// backspace reshape the query; Value waits untouched until a key accepts.
func TestTypingEditsQueryNeverTheBuffer(t *testing.T) {
	c, _ := newSearchInput(t, "go test", "go run")
	c.Value = "draft"
	c.Cursor = len("draft")
	require.True(t, c.BeginSearch())

	typeQuery(c, "go t")
	c.Handle(&components.EventContext{}, key(xui.KeyBackspace))

	require.True(t, c.SearchActive())
	require.Equal(t, "draft", c.Value)
	require.Equal(t, len("draft"), c.Cursor)
}

// TestSearchStepsClampAtBothEnds: Ctrl+R walks older until the oldest match,
// Ctrl+S walks newer until the newest; neither ever leaves the match list.
func TestSearchStepsClampAtBothEnds(t *testing.T) {
	c, _ := newSearchInput(t, "one build", "two build", "three build")
	require.True(t, c.BeginSearch())
	typeQuery(c, "build")

	// Newest match first.
	acceptByEscape(c)
	require.Equal(t, "three build", c.Value)
	require.False(t, c.SearchActive())

	// Step older twice, then past the oldest match: it clamps.
	require.True(t, c.BeginSearch())
	typeQuery(c, "build")
	require.True(t, c.SearchOlder())
	require.True(t, c.SearchOlder())
	require.False(t, c.SearchOlder(), "the oldest match is the floor")
	acceptByEscape(c)
	require.Equal(t, "one build", c.Value)

	// Step newer back, then past the newest: it floors.
	require.True(t, c.BeginSearch())
	typeQuery(c, "build")
	require.True(t, c.SearchOlder())
	require.True(t, c.SearchOlder())
	require.True(t, c.SearchNewer())
	require.True(t, c.SearchNewer())
	require.False(t, c.SearchNewer(), "the newest match is the ceiling")
	acceptByEscape(c)
	require.Equal(t, "three build", c.Value)
}

// TestQueryRefineLandsOnNewestMatch: every query edit resets the position to
// the newest match, the way bash restarts the walk on each keystroke.
func TestQueryRefineLandsOnNewestMatch(t *testing.T) {
	c, _ := newSearchInput(t, "go run", "go test")
	require.True(t, c.BeginSearch())

	typeQuery(c, "go")
	require.True(t, c.SearchOlder()) // "go run", the older of the two
	typeQuery(c, " t")               // refine: back to the newest "go test"

	acceptByEscape(c)
	require.Equal(t, "go test", c.Value)
}

// TestEnterSubmitsMatchThroughOnSubmit: bare Enter sends the match through
// the one submit path, exits the mode, and leaves the match in the buffer.
func TestEnterSubmitsMatchThroughOnSubmit(t *testing.T) {
	c, submitted := newSearchInput(t, "hello world", "hello there")
	require.True(t, c.BeginSearch())
	typeQuery(c, "there")

	c.Handle(&components.EventContext{}, key(xui.KeyEnter))

	require.False(t, c.SearchActive())
	require.Equal(t, []string{"hello there"}, *submitted)
	require.Equal(t, "hello there", c.Value)
	require.Equal(t, len("hello there"), c.Cursor)
}

// TestEnterWithoutMatchStaysInTheMode: nothing to send, so Enter is consumed
// and changes nothing.
func TestEnterWithoutMatchStaysInTheMode(t *testing.T) {
	c, submitted := newSearchInput(t, "hello")
	require.True(t, c.BeginSearch())
	typeQuery(c, "zzz")

	ctx := &components.EventContext{}
	c.Handle(ctx, key(xui.KeyEnter))

	require.True(t, c.SearchActive(), "Enter with no match must not exit the mode")
	require.True(t, ctx.Consume)
	require.Empty(t, *submitted)
	require.Empty(t, c.Value)
}

// TestEscapeAcceptsWithoutSubmit: Esc keeps the match in the buffer for
// editing, caret at its end, and never sends.
func TestEscapeAcceptsWithoutSubmit(t *testing.T) {
	c, submitted := newSearchInput(t, "hello world", "hello there")
	require.True(t, c.BeginSearch())
	typeQuery(c, "world")

	ctx := &components.EventContext{}
	c.Handle(ctx, key(xui.KeyEscape))

	require.False(t, c.SearchActive())
	require.True(t, ctx.Consume, "the pane ladder must never see Esc mid-search")
	require.Equal(t, "hello world", c.Value)
	require.Equal(t, len("hello world"), c.Cursor)
	require.Empty(t, *submitted)
}

// TestEscapeWithoutMatchRestoresDraft: with no match to accept, leaving the
// mode gives the draft back.
func TestEscapeWithoutMatchRestoresDraft(t *testing.T) {
	c, _ := newSearchInput(t, "hello")
	c.Value = "draft"
	require.True(t, c.BeginSearch())
	typeQuery(c, "zzz")

	acceptByEscape(c)

	require.False(t, c.SearchActive())
	require.Equal(t, "draft", c.Value)
}

// TestNavigationExitsWithMatchThenMovesTheCaret: arrows, Home and End end the
// mode accepting the match, then apply as the caret key they normally are.
func TestNavigationExitsWithMatchThenMovesTheCaret(t *testing.T) {
	c, _ := newSearchInput(t, "abcde")
	require.True(t, c.BeginSearch())
	typeQuery(c, "abc")

	c.Handle(&components.EventContext{}, key(xui.KeyLeft))

	require.False(t, c.SearchActive())
	require.Equal(t, "abcde", c.Value)
	require.Equal(t, len("abcde")-1, c.Cursor, "Left must move the caret after the accept")
}

// TestTabAcceptsAndConsumes: Tab keeps the match too, but swallows the key so
// the pane cannot read it as the mode toggle.
func TestTabAcceptsAndConsumes(t *testing.T) {
	c, submitted := newSearchInput(t, "hello world")
	require.True(t, c.BeginSearch())
	typeQuery(c, "world")

	ctx := &components.EventContext{}
	c.Handle(ctx, key(xui.KeyTab))

	require.False(t, c.SearchActive())
	require.True(t, ctx.Consume)
	require.Equal(t, "hello world", c.Value)
	require.Empty(t, *submitted)
}

// TestEditingChordsAreInertWhileSearching: the body is a preview during the
// mode, so the line-editing chords must not reach the draft underneath.
func TestEditingChordsAreInertWhileSearching(t *testing.T) {
	c, _ := newSearchInput(t, "draft line")
	c.Value, c.Cursor = "typed draft", len("typed draft")
	require.True(t, c.BeginSearch())
	typeQuery(c, "draf")

	for _, r := range []rune{'u', 'a', 'x'} {
		ctx := &components.EventContext{}
		c.Handle(ctx, xui.KeyEvent{Code: xui.KeyRune, Rune: r, Mods: xui.ModCtrl, Press: true})
		require.False(t, ctx.Consume, "editing chords bubble on unchanged while searching")
		require.True(t, c.SearchActive())
	}
	require.Equal(t, "typed draft", c.Value, "the draft must survive the chords untouched")
}

// TestSearchPreviewOwnsTheBody: while reverse-i-search is on, the body is a
// preview — the match the mode sits on, or the muted "no matches" line. The
// draft placeholder must not paint over either, even though the draft (Value)
// it keys on is often empty at that moment.
func TestSearchPreviewOwnsTheBody(t *testing.T) {
	c, _ := newSearchInput(t, "go test ./...")
	c.Theme = components.DefaultTheme()
	c.Placeholder = "Ask anything..."

	require.True(t, c.BeginSearch())
	typeQuery(c, "go")

	s := c.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 12}, Method: xui.WidthUnicode})
	row := rowString(s, 1)
	require.NotContains(t, row, "Ask anything", "placeholder painted over the match preview")
	require.Contains(t, row, "go test ./...", "match preview missing from the body")

	// No hits hand the body to the "no matches" line — same rule.
	typeQuery(c, "zz")
	s = c.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 12}, Method: xui.WidthUnicode})
	row = rowString(s, 1)
	require.NotContains(t, row, "Ask anything", "placeholder painted over the no-matches line")
	require.Contains(t, row, "no matches", "no-matches line missing from the body")
}
