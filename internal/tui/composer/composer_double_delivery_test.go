package composer

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
)

// App.dispatch delivers a key to the focused widget first and bubbles the
// unconsumed remainder up to the editor root, whose ladder ends back in the
// composer. The composer must not re-deliver to the widget that already
// refused the event: an event reaches a widget exactly once. Today every
// mutating branch consumes, so a re-run is latent corruption — e.g. a
// backspace applied twice.

func TestComposerSkipsRedeliveryToChat(t *testing.T) {
	c := newTestPane()
	c.Wire(nil, nil, nil, "", &fakeBus{}, &fakeFocus{})

	c.Chat.Value = "ab"
	c.Chat.Cursor = 2
	ctx := &components.EventContext{DeliveredTo: &c.Chat}

	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyBackspace, Press: true})

	require.Equal(t, "ab", c.Chat.Value, "the chat already refused this key; it must not run again")
}

func TestComposerDeliversToChatWhenNotAlreadyDelivered(t *testing.T) {
	c := newTestPane()
	c.Wire(nil, nil, nil, "", &fakeBus{}, &fakeFocus{})

	c.Chat.Value = "ab"
	c.Chat.Cursor = 2

	c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyBackspace, Press: true})

	require.Equal(t, "a", c.Chat.Value, "editor-root focus still routes keys into the chat once")
}

func TestComposerSkipsRedeliveryToPalette(t *testing.T) {
	c := newTestPane()
	c.Wire(nil, nil, nil, "", &fakeBus{}, &fakeFocus{})

	c.palette.Show()
	c.palette.Query = "x"
	c.palette.Cursor = 1
	ctx := &components.EventContext{DeliveredTo: &c.palette}

	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyBackspace, Press: true})

	require.Equal(t, "x", c.palette.Query, "the palette already refused this key; it must not run again")
}

func TestComposerDeliversToPaletteWhenNotAlreadyDelivered(t *testing.T) {
	c := newTestPane()
	c.Wire(nil, nil, nil, "", &fakeBus{}, &fakeFocus{})

	c.palette.Show()
	c.palette.Query = "x"
	c.palette.Cursor = 1

	c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyBackspace, Press: true})

	require.Empty(t, c.palette.Query, "keys still reach an open palette through the composer")
}
