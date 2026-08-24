package composer

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/tui/controller"
)

type fakeBus struct {
	published controller.Msg
	drained   bool
	refreshed bool
}

func (b *fakeBus) Publish(m controller.Msg) { b.published = m }
func (b *fakeBus) DrainNow()                { b.drained = true }
func (b *fakeBus) RequestRefresh()          { b.refreshed = true }

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
