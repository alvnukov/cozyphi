package overlays

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

func motion(x, y int) xui.MouseEvent {
	return xui.MouseEvent{X: x, Y: y, Action: xui.MouseMotion}
}

// The modal ask is not a hit-tested widget, so hover rides the motion events
// the modal already swallows: the option under the pointer tints alone, a
// redraw is asked for only when the hovered option changes, and motion onto
// prose clears the tint.
func TestAskOptionHoverTint(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	beginTestPermissionAsk(t, o)
	body, answer := o.perm.askRows(o.theme, askInnerWidth(80), 0)
	approveY := rowContaining(t, body, "Approve")

	ctx := &components.EventContext{}
	require.True(t, o.HandleAskMouse(ctx, motion(4, approveY)))
	assert.True(t, ctx.Consume, "a modal is modal for motion too")
	assert.True(t, ctx.Redraw, "a new hover asks for a frame")

	ctx = &components.EventContext{}
	require.True(t, o.HandleAskMouse(ctx, motion(6, approveY)))
	assert.False(t, ctx.Redraw, "the same option does not redraw")

	h, _ := o.PreferredBottomHeight(80, 0)
	panel, _ := o.DrawBottom(components.DrawContext{}, 80, h)
	want := o.theme.BackgroundElement.Bg
	row0, row1, ok := askHoverRows(len(body), answer,
		o.perm.optionBlocks(o.theme, askPrimary(o.theme), askInnerWidth(80), 0), o.askHover)
	require.True(t, ok, "the hovered option must have rows")
	require.Positive(t, row0, "options sit after the prose")
	for y := row0; y < row1; y++ {
		for x := 1; x < 79; x++ {
			assert.Equal(t, want, panel.Buffer[(y+1)*80+x].Style.Bg, "option row %d col %d", y, x)
		}
	}
	for x := 1; x < 79; x++ {
		assert.NotEqual(t, want, panel.Buffer[80+x].Style.Bg, "the header row stays quiet")
	}

	ctx = &components.EventContext{}
	require.True(t, o.HandleAskMouse(ctx, motion(4, 10+1)))
	assert.Equal(t, -1, o.askHover, "motion on prose clears the hover")
	assert.True(t, ctx.Redraw, "clearing asks for a frame")
	cleared, _ := o.DrawBottom(components.DrawContext{}, 80, h)
	for y := row0; y < row1; y++ {
		assert.NotEqual(t, want, cleared.Buffer[(y+1)*80+4].Style.Bg, "cleared row %d", y)
	}
}
