package app

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
)

// recordingWidget counts deliveries and optionally consumes, so tests can
// play both the widget that claims an event and the one that refuses it.
type recordingWidget struct {
	deliveries int
	consume    bool
}

func (w *recordingWidget) Handle(ctx *components.EventContext, _ xui.Event) {
	w.deliveries++
	if w.consume {
		ctx.ConsumeAndRedraw()
	}
}

func (*recordingWidget) Draw(components.DrawContext) components.Surface {
	return components.Surface{}
}

// TestDispatchMarksFocusedDelivery: when the focused widget refuses a key,
// dispatch marks it in ctx.DeliveredTo before bubbling to the root, so
// forwarding parents along the root ladder can skip re-delivering to it.
func TestDispatchMarksFocusedDelivery(t *testing.T) {
	focused := &recordingWidget{}
	root := &recordingWidget{}
	a := NewApp(nil)
	a.root = root
	a.focused = focused

	ctx := &components.EventContext{}
	a.dispatch(ctx, xui.KeyEvent{Code: xui.KeyRune, Rune: 'x', Press: true})

	require.Equal(t, 1, focused.deliveries)
	require.Equal(t, 1, root.deliveries, "unconsumed keys still bubble to root")
	require.Same(t, components.Widget(focused), ctx.DeliveredTo)
}

// TestDispatchConsumeStopsBubbling: a consumed event never reaches the root,
// and DeliveredTo stays zero — there is nothing left to guard against.
func TestDispatchConsumeStopsBubbling(t *testing.T) {
	focused := &recordingWidget{consume: true}
	root := &recordingWidget{}
	a := NewApp(nil)
	a.root = root
	a.focused = focused

	ctx := &components.EventContext{}
	a.dispatch(ctx, xui.KeyEvent{Code: xui.KeyRune, Rune: 'x', Press: true})

	require.Equal(t, 1, focused.deliveries)
	require.Equal(t, 0, root.deliveries)
	require.Nil(t, ctx.DeliveredTo)
}

// TestDispatchNoFocusedDeliversToRootOnly: when focus is the root itself the
// root is the sole delivery and DeliveredTo stays zero — the guard only
// records a first delivery that actually happened.
func TestDispatchNoFocusedDeliversToRootOnly(t *testing.T) {
	root := &recordingWidget{}
	a := NewApp(nil)
	a.root = root
	a.focused = root

	ctx := &components.EventContext{}
	a.dispatch(ctx, xui.KeyEvent{Code: xui.KeyRune, Rune: 'x', Press: true})

	require.Equal(t, 1, root.deliveries)
	require.Nil(t, ctx.DeliveredTo)
}
