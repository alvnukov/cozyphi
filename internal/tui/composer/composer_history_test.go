package composer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/mention"
	"github.com/alvnukov/cozyphi/internal/history"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

// TestComposerSubmitAppendsHistory: a submit through the wired pane lands in
// the prompt history store, so the next session (and the next Up) sees it.
func TestComposerSubmitAppendsHistory(t *testing.T) {
	h := history.Open("")
	c := NewComposerPane(components.DefaultTheme(), "model", "/tmp", h)
	bus := &fakeBus{}
	c.Wire(nil, nil, nil, "", bus, &fakeFocus{})

	c.Chat.OnSubmit("  hello  ")

	require.Equal(t, controller.SubmitMsg{Text: "  hello  "}, bus.published)
	require.Equal(t, 1, h.Len())
	got, ok := h.Prev("")
	require.True(t, ok)
	require.Equal(t, "hello", got)
}

// TestComposerClearInputResetsHistoryWalk: after the submitter clears the
// composer, Up starts from the newest entry again, not mid-walk.
func TestComposerClearInputResetsHistoryWalk(t *testing.T) {
	h := history.Open("")
	h.Append("old")
	c := NewComposerPane(components.DefaultTheme(), "model", "/tmp", h)
	c.Wire(nil, nil, nil, "", &fakeBus{}, &fakeFocus{})

	got, ok := h.Prev("")
	require.True(t, ok)
	require.Equal(t, "old", got)
	c.Chat.Value = got

	c.ClearInput()

	require.Empty(t, c.Chat.Value)
	_, ok = h.Next("old")
	require.False(t, ok, "ClearInput must reset the history walk")
}

// TestComposerNilHistoryStillSubmits: a store that failed to open degrades to
// plain submission without history.
func TestComposerNilHistoryStillSubmits(t *testing.T) {
	c := NewComposerPane(components.DefaultTheme(), "model", "/tmp", nil)
	bus := &fakeBus{}
	c.Wire(nil, nil, nil, "", bus, &fakeFocus{})

	c.Chat.OnSubmit("hello")

	require.Equal(t, controller.SubmitMsg{Text: "hello"}, bus.published)
}

// TestComposerAcceptSlashRecordsHistory: a no-arg command accepted from the
// / picker takes the same submit path as a typed command — Chat.OnSubmit —
// so it lands in the prompt history; Up from "/" recalls it as a slash walk.
func TestComposerAcceptSlashRecordsHistory(t *testing.T) {
	h := history.Open("")
	c := NewComposerPane(components.DefaultTheme(), "model", "/tmp", h)
	bus := &fakeBus{}
	c.Wire(nil, nil, nil, "", bus, &fakeFocus{})

	c.Chat.Value = "/cle"
	c.Chat.Cursor = len("/cle")
	c.slash.OnAccept(mention.Item{Path: "clear"})

	require.Equal(t, controller.SubmitMsg{Text: "/clear"}, bus.published)
	require.Equal(t, []string{"/clear"}, h.Entries())
	got, ok := h.Prev("/")
	require.True(t, ok)
	require.Equal(t, "/clear", got)
}
