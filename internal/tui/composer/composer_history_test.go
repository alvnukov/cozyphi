package composer

import (
	"testing"

	"github.com/pulseaiclub/xui"
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

// ctrlR and ctrlS build the reverse-i-search chords the pane resolves
// through the keys table.
func ctrlR() xui.KeyEvent {
	return xui.KeyEvent{Code: xui.KeyRune, Rune: 'r', Mods: xui.ModCtrl, Press: true}
}

func ctrlS() xui.KeyEvent {
	return xui.KeyEvent{Code: xui.KeyRune, Rune: 's', Mods: xui.ModCtrl, Press: true}
}

func typeInto(c *ComposerPane, text string) {
	for _, r := range text {
		c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyRune, Rune: r, Press: true})
	}
}

// TestComposerCtrlREntersSearchAndRepeatsStepOlder: the chord arrives at the
// pane, enters the mode through the default binding, typing filters, and a
// repeated Ctrl+R walks the matches older until Enter submits the one it
// landed on through the one submit path.
func TestComposerCtrlREntersSearchAndRepeatsStepOlder(t *testing.T) {
	h := history.Open("")
	h.Append("one build")
	h.Append("two build")
	c := NewComposerPane(components.DefaultTheme(), "model", "/tmp", h)
	bus := &fakeBus{}
	c.Wire(nil, nil, nil, "", bus, &fakeFocus{})

	c.Handle(&components.EventContext{}, ctrlR())
	require.True(t, c.Chat.SearchActive(), "Ctrl+R must enter reverse-i-search")
	typeInto(c, "build")

	c.Handle(&components.EventContext{}, ctrlR()) // step older
	c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyEnter, Press: true})

	require.False(t, c.Chat.SearchActive())
	require.Equal(t, controller.SubmitMsg{Text: "one build"}, bus.published)
	require.Equal(t, "one build", c.Chat.Value)
}

// TestComposerCtrlGAbortsSearchNotVoice: while the mode is on, the voice
// chord is its abort (readline's Ctrl+G) and never reaches the microphone;
// once it is off, the same chord toggles voice exactly as before.
func TestComposerCtrlGAbortsSearchNotVoice(t *testing.T) {
	h := history.Open("")
	h.Append("hello world")
	c := NewComposerPane(components.DefaultTheme(), "model", "/tmp", h)
	c.Wire(nil, nil, nil, "", &fakeBus{}, &fakeFocus{})
	v := &fakeVoice{}
	c.SetVoice(v)

	c.Chat.Value = "draft"
	c.Handle(&components.EventContext{}, ctrlR())
	typeInto(c, "hello")
	c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyRune, Rune: 'g', Mods: xui.ModCtrl, Press: true})

	require.False(t, c.Chat.SearchActive(), "Ctrl+G must abort the search")
	require.Zero(t, v.starts, "the abort must not reach the microphone")
	require.Equal(t, "draft", c.Chat.Value, "the abort restores the draft")

	c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyRune, Rune: 'g', Mods: xui.ModCtrl, Press: true})
	require.Equal(t, 1, v.starts, "Ctrl+G toggles voice again once no search is on")
}

// TestComposerCtrlSOnlyAppliesWhileSearching: with the mode off the chord
// stays unconsumed; with it on, Ctrl+S steps newer and floors at the newest
// match.
func TestComposerCtrlSOnlyAppliesWhileSearching(t *testing.T) {
	h := history.Open("")
	h.Append("one build")
	h.Append("two build")
	c := NewComposerPane(components.DefaultTheme(), "model", "/tmp", h)
	bus := &fakeBus{}
	c.Wire(nil, nil, nil, "", bus, &fakeFocus{})

	ctx := &components.EventContext{}
	c.Handle(ctx, ctrlS())
	require.False(t, ctx.Consume, "Ctrl+S without an active search is not ours to take")
	require.False(t, c.Chat.SearchActive())

	c.Handle(&components.EventContext{}, ctrlR())
	typeInto(c, "build")
	c.Handle(&components.EventContext{}, ctrlR()) // older: "one build"

	ctx = &components.EventContext{}
	c.Handle(ctx, ctrlS()) // newer: "two build"
	require.True(t, ctx.Consume)
	require.True(t, c.Chat.SearchActive())

	c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyEnter, Press: true})
	require.Equal(t, controller.SubmitMsg{Text: "two build"}, bus.published)
}

// TestComposerCtrlRUnderOpenPaletteIsNotOurs: while the palette owns the
// keyboard the history-search chord must not start a mode beneath it.
func TestComposerCtrlRUnderOpenPaletteIsNotOurs(t *testing.T) {
	h := history.Open("")
	h.Append("hello world")
	c := NewComposerPane(components.DefaultTheme(), "model", "/tmp", h)
	c.Wire(nil, nil, nil, "", &fakeBus{}, &fakeFocus{})

	c.palette.Show()
	c.Handle(&components.EventContext{}, ctrlR())

	require.False(t, c.Chat.SearchActive())
	require.True(t, c.palette.Open, "the palette keeps the keyboard")
}
