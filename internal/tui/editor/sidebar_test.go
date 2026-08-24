package editor

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/project"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tui/controller"
	"github.com/pulseaiclub/phi/internal/tui/sidebar"
)

func newTestEditor(t *testing.T) *Editor {
	t.Helper()
	return newTestEditorAt(t, t.TempDir(), t.TempDir())
}

func newTestEditorAt(t *testing.T, home, cwd string) *Editor {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PHI_MODEL", "test-model")
	t.Setenv("PHI_API_KEY", "test-key")
	t.Setenv("PHI_BASE_URL", "http://127.0.0.1:9")

	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())

	bus := controller.NewBus(nil)
	ctrl, err := controller.NewController(bus, proj, cwd, "")
	require.NoError(t, err)
	return NewEditor(nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "m", "", 1000, nil, nil)
}

func sidebarText(e *Editor) string {
	return components.SurfaceText(e.sidebar.Draw(components.DrawContext{
		Max:    components.Size{Width: sidebar.Width, Height: 24},
		Method: xui.WidthUnicode,
	}))
}

// TestEditorCtrlOTogglesSidebar pins the default-visible binding: Draw reserves
// right-hand columns on wide terminals, narrow terminals suppress the panel,
// and Ctrl+O hides it immediately.
func TestEditorCtrlOTogglesSidebar(t *testing.T) {
	e := newTestEditor(t)
	require.True(t, e.sidebar.Visible())

	root := e.Draw(components.DrawContext{
		Max:    components.Size{Width: 120, Height: 30},
		Method: xui.WidthUnicode,
	})
	require.Len(t, root.Children, 4, "list, chat, footer, sidebar")
	assert.Equal(t, components.Point{X: 120 - sidebar.Width, Y: 0}, root.Children[3].Origin)
	assert.Equal(t, sidebar.Width, root.Children[3].Surface.Size.Width)
	assert.Equal(t, 30, root.Children[3].Surface.Size.Height, "panel spans the full height")

	narrow := e.Draw(components.DrawContext{
		Max:    components.Size{Width: 80, Height: 30},
		Method: xui.WidthUnicode,
	})
	assert.Len(t, narrow.Children, 3, "80-column terminal suppresses the panel")

	ctx := &components.EventContext{}
	e.Handle(ctx, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'o', Mods: xui.ModCtrl})
	require.False(t, e.sidebar.Visible())
	assert.True(t, ctx.Consume && ctx.Redraw)
}

func TestEditorPersistsSidebarVisibilityAcrossInstances(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()

	first := newTestEditorAt(t, home, cwd)
	require.True(t, first.sidebar.Visible(), "missing preference defaults to visible")
	first.Handle(
		&components.EventContext{},
		xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'o', Mods: xui.ModCtrl},
	)
	require.False(t, first.sidebar.Visible())

	second := newTestEditorAt(t, home, cwd)
	require.False(t, second.sidebar.Visible(), "new instance restores the last hidden state")
	second.Handle(
		&components.EventContext{},
		xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'o', Mods: xui.ModCtrl},
	)

	third := newTestEditorAt(t, home, cwd)
	require.True(t, third.sidebar.Visible(), "new instance restores the last visible state")
}

// TestEditorSidebarFollowsUsageAndClear: turn usage reported by the transcript
// reaches the panel, and /clear drops it again.
func TestEditorSidebarFollowsUsageAndClear(t *testing.T) {
	e := newTestEditor(t)

	e.transcript.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
		ID:    "m1",
		State: session.StateComplete,
		Usage: session.TokenUsage{PromptTokens: 500, TotalTokens: 600},
	}})
	e.transcript.Sync()

	assert.Contains(t, sidebarText(e), "↑500", "per-turn usage reaches the panel")
	assert.Contains(t, sidebarText(e), "50%", "context fill is derived from the window")

	e.ClearSession()
	assert.Contains(t, sidebarText(e), "awaiting usage", "/clear resets the panel")
}
