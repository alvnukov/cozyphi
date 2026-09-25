package block_test

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/session"
)

func drawAside(a *block.AsideBlock) (components.Surface, []string) {
	s := a.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 20}, Method: xui.WidthUnicode})
	return s, strings.Split(components.SurfaceText(s), "\n")
}

// The title carries the question, and names the message it was about only
// when that was not the end of the context.
func TestAsideBlockTitleNamesTheAnchorWhenThereIsOne(t *testing.T) {
	a := &block.AsideBlock{Question: "why this?", Answer: "because", Expanded: true, State: session.StateComplete}
	_, rows := drawAside(a)
	assert.Contains(t, rows[0], "btw why this?")
	assert.NotContains(t, rows[0], "re:")
	assert.Contains(t, strings.Join(rows[1:], "\n"), "because")

	a.AnchorPreview = "answer the first one"
	_, rows = drawAside(a)
	assert.Contains(t, rows[0], "re: answer the first one")
}

// The row takes its color from the theme, the light one as much as the dark
// one: the gutter and the btw label are the theme's Aside.
func TestAsideBlockPaintsWithTheThemeAsideColor(t *testing.T) {
	light, dark := components.VSLightTheme(), components.DarkTheme()
	require.NotEqual(t, light.Aside, dark.Aside)
	for _, th := range []components.Theme{light, dark} {
		a := &block.AsideBlock{Question: "q", Answer: "a", Expanded: true, Theme: th, State: session.StateComplete}
		s, rows := drawAside(a)
		for y := range s.Size.Height {
			assert.Equal(t, th.Aside.Fg, s.Buffer[y*s.Size.Width].Style.Fg, "%s: gutter of row %d", th.Name, y)
		}
		label := strings.Index(rows[0], "btw")
		require.GreaterOrEqual(t, label, 0)
		x := xui.StringWidth(rows[0][:label], xui.WidthUnicode)
		assert.Equal(t, th.Aside.Fg, s.Buffer[x].Style.Fg, "%s: the btw label", th.Name)
	}
}

// A click on the title folds the answer away and a second one brings it
// back; the hover lights the title and the tooltip says what a click does.
func TestAsideBlockFoldsOnATitleClick(t *testing.T) {
	var toggled []bool
	a := &block.AsideBlock{
		Question: "q", Answer: "the answer", Expanded: true, State: session.StateComplete,
		Theme:    components.DefaultTheme(),
		OnToggle: func(expanded bool) { toggled = append(toggled, expanded) },
	}
	_, rows := drawAside(a)
	require.Contains(t, strings.Join(rows, "\n"), "the answer")
	tip, ok := a.HoverTooltip(1, 0)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(tip, "fold"))
	_, ok = a.HoverTooltip(1, 1)
	assert.False(t, ok, "the answer itself is no fold")
	assert.Equal(t, components.ShapePointer, a.PointerShape(1, 0))

	press := xui.MouseEvent{X: 4, Y: 0, Button: xui.MouseLeft, Action: xui.MousePress}
	a.Handle(&components.EventContext{}, press)
	assert.False(t, a.Expanded)
	_, rows = drawAside(a)
	assert.NotContains(t, strings.Join(rows, "\n"), "the answer")
	tip, _ = a.HoverTooltip(1, 0)
	assert.True(t, strings.HasPrefix(tip, "unfold"))

	a.Handle(&components.EventContext{}, press)
	assert.True(t, a.Expanded)
	assert.Equal(t, []bool{false, true}, toggled)

	th := components.DefaultTheme()
	s := a.Draw(hoveredCtx(a))
	assert.Equal(t, th.BackgroundElement.Bg, hoverTint(t, s, 0), "the hovered title lights up")
	assert.NotEqual(t, th.BackgroundElement.Bg, hoverTint(t, s, 1), "the answer does not")

	assert.True(t, a.CollapseOnClick())
	assert.False(t, a.CollapseOnClick(), "a folded row has nothing more to fold")
}

// Before the first words arrive there is nothing to fold, so the title does
// not pretend to be a toggle.
func TestAsideBlockWithoutAnAnswerIsNoToggle(t *testing.T) {
	a := &block.AsideBlock{Question: "q", Expanded: true, State: session.StateStreaming}
	_, ok := a.HoverTooltip(1, 0)
	assert.False(t, ok)
	assert.Equal(t, components.ShapeText, a.PointerShape(1, 0))
	assert.False(t, a.CollapseOnClick())

	a.State, a.Error = session.StateError, "provider said no"
	_, rows := drawAside(a)
	assert.Contains(t, rows[0], "(failed)")
	assert.Contains(t, strings.Join(rows, "\n"), "provider said no")
}

// A copy of the row carries what the row shows: the question, the answer so
// far and the error that ended it.
func TestAsideBlockCopyKeepsTheError(t *testing.T) {
	a := &block.AsideBlock{Question: "q", Answer: "half", Error: "provider said no", State: session.StateError}
	assert.Equal(t, "btw: q\n\nhalf\n\nprovider said no", a.CopyText())
}
