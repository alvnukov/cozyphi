package composer

import (
	"sync"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

// Mention searches publish from background goroutines
// (scheduleMentionSearch), so the fake mirrors the real Bus's mutex.
type fakeBus struct {
	mu        sync.Mutex
	published controller.Msg
	drained   bool
	refreshed bool
}

func (b *fakeBus) Publish(m controller.Msg) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = m
}

func (b *fakeBus) DrainNow() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.drained = true
}

func (b *fakeBus) RequestRefresh() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refreshed = true
}

type fakeFocus struct {
	focusedEditor bool
	focusedWidget components.Widget
}

func (f *fakeFocus) FocusEditor()              { f.focusedEditor = true }
func (f *fakeFocus) Focus(w components.Widget) { f.focusedWidget = w }

func newTestPane() *ComposerPane {
	return NewComposerPane(components.DefaultTheme(), "model", "/tmp", nil)
}

func TestComposerWireSubmitsThroughBus(t *testing.T) {
	c := newTestPane()
	bus := &fakeBus{}
	c.Wire(nil, nil, nil, "", bus, &fakeFocus{})

	c.Chat.OnSubmit("hello")

	require.Equal(t, controller.SubmitMsg{Text: "hello"}, bus.published)
	require.True(t, bus.drained)
}

func TestComposerWireOnChangeRequestsRefresh(t *testing.T) {
	c := newTestPane()
	bus := &fakeBus{}
	c.Wire(nil, nil, nil, "", bus, &fakeFocus{})

	c.Chat.OnChange("typing")

	require.True(t, bus.refreshed)
}

func TestComposerFocusChatFocusesWidget(t *testing.T) {
	c := newTestPane()
	focus := &fakeFocus{}
	c.Wire(nil, nil, nil, "", &fakeBus{}, focus)

	c.FocusChat()

	require.NotNil(t, focus.focusedWidget)
}

// TestComposerPlaceholderPosture: the placeholder mirrors the posture — the
// ask hint by default, the shell hint while a "!" prefix is active.
func TestComposerPlaceholderPosture(t *testing.T) {
	c := newTestPane()
	require.Equal(t, "Ask anything...", c.Chat.Placeholder)

	c.SetBashBorderActive(true)
	require.Equal(t, "Run a command...", c.Chat.Placeholder)

	c.SetBashBorderActive(false)
	require.Equal(t, "Ask anything...", c.Chat.Placeholder)
}

// Focus landing on the composer routes to the inner widget that owns input:
// the palette when open, the chat input otherwise. Modal-overlaid focus is
// kept away from composer widgets by the Focuser adapter, not here.
func TestComposerFocusEventRoutesToInput(t *testing.T) {
	c := newTestPane()
	focus := &fakeFocus{}
	c.Wire(nil, nil, nil, "", &fakeBus{}, focus)

	c.Handle(&components.EventContext{}, xui.FocusEvent{Focused: true})
	require.Same(t, &c.Chat, focus.focusedWidget)

	c.palette.Show()
	focus.focusedWidget = nil
	c.Handle(&components.EventContext{}, xui.FocusEvent{Focused: true})
	require.Same(t, &c.palette, focus.focusedWidget)
}

// TestComposerCtrlKRefreshesPalette: every open rebuilds the root list via the
// installed supplier, so usage-ranking changes show on the next open instead
// of at startup.
func TestComposerCtrlKRefreshesPalette(t *testing.T) {
	c := newTestPane()
	focus := &fakeFocus{}
	c.Wire(nil, nil, nil, "", &fakeBus{}, focus)

	calls := 0
	c.SetPaletteRefresh(func() []palette.PaletteCommand {
		calls++
		return []palette.PaletteCommand{{ID: "fresh", Verb: "freshly built"}}
	})

	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyRune, Rune: 'k', Mods: xui.ModCtrl, Press: true})
	require.True(t, c.palette.Open)
	require.Equal(t, 1, calls)
	require.Len(t, c.palette.Commands, 1)
	require.Equal(t, "fresh", c.palette.Commands[0].ID)
}

// TestComposerArrowsDoNotReopenDismissedSlash: after Escape dismisses the
// slash picker, Up/Down must not bounce the caret (Home/End on a single line)
// nor re-open the picker — the picker is typing-driven, not caret-driven.
func TestComposerArrowsDoNotReopenDismissedSlash(t *testing.T) {
	c := wiredCmdPane(t)
	c.Chat.Value = "/"
	c.Chat.Cursor = 1
	notifySlashBoth(c)
	require.True(t, c.slash.Open)

	c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyEscape, Press: true})
	require.False(t, c.slash.Open)
	require.False(t, c.Chat.SlashOpen)

	c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyUp, Press: true})
	require.Equal(t, 1, c.Chat.Cursor, "Up must not bounce the caret to 0")
	require.False(t, c.slash.Open, "Up must not re-open the slash picker")

	c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyDown, Press: true})
	require.Equal(t, 1, c.Chat.Cursor, "Down must not bounce the caret to the end")
	require.False(t, c.slash.Open, "Down must not re-open the slash picker")
	require.False(t, c.Chat.SlashOpen)
}

// stubSubmitter stands in for the submit side of the Esc ladder: busy like a
// run in flight, with or without a queued prompt to hand back.
type stubSubmitter struct {
	busy       bool
	recallText string
	recallOK   bool
}

func (s *stubSubmitter) CanSubmit() bool              { return !s.busy }
func (*stubSubmitter) SyncBashBorder(string)          {}
func (s *stubSubmitter) RecallQueued() (string, bool) { return s.recallText, s.recallOK }

// TestComposerEscRecallsQueuedPromptIntoChat: while the submit side is busy
// and a prompt is queued, Esc hands the queued text back to the input —
// caret at the end, nothing published to the bus, run untouched. A draft
// typed in the meantime survives as its own line above the recalled text.
func TestComposerEscRecallsQueuedPromptIntoChat(t *testing.T) {
	c := newTestPane()
	bus := &fakeBus{}
	sub := &stubSubmitter{busy: true, recallText: "second", recallOK: true}
	c.Wire(nil, sub, nil, "", bus, &fakeFocus{})

	c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyEscape, Press: true})

	require.Equal(t, "second", c.Chat.Value)
	require.Equal(t, len("second"), c.Chat.Cursor)
	require.Nil(t, bus.published, "recall must not cancel the run")
	require.False(t, bus.drained)

	// A half-typed draft is not eaten: the recalled prompt lands below it.
	c.Chat.Value = "draft"
	c.Chat.Cursor = len("draft")
	c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyEscape, Press: true})

	require.Equal(t, "draft\nsecond", c.Chat.Value)
	require.Equal(t, len("draft\nsecond"), c.Chat.Cursor)
}

// TestComposerEscCancelsRunWhenQueueEmpty: with nothing queued, Esc while
// busy keeps its old meaning — stop the run.
func TestComposerEscCancelsRunWhenQueueEmpty(t *testing.T) {
	c := newTestPane()
	bus := &fakeBus{}
	c.Wire(nil, &stubSubmitter{busy: true}, nil, "", bus, &fakeFocus{})

	c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyEscape, Press: true})

	require.Equal(t, controller.CancelStreamMsg{}, bus.published)
	require.True(t, bus.drained)
	require.Empty(t, c.Chat.Value, "cancel must not touch the draft")
}
