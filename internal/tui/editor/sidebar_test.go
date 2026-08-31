package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/sidebar"
)

func newTestEditor(t *testing.T) *Editor {
	t.Helper()
	return newTestEditorAt(t, t.TempDir(), t.TempDir())
}

func newTestEditorAt(t *testing.T, home, cwd string) *Editor {
	t.Helper()
	return newTestEditorResuming(t, home, cwd, "")
}

// newTestEditorResuming builds the shell on a session file that already exists,
// which is how a test gets an editor with durable state behind it.
func newTestEditorResuming(t *testing.T, home, cwd, resumePath string) *Editor {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")

	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())

	bus := controller.NewBus(nil)
	ctrl, err := controller.NewController(bus, proj, cwd, resumePath)
	require.NoError(t, err)
	return NewEditor(nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "m", "", 1000, nil, nil)
}

func sidebarText(e *Editor) string {
	return components.SurfaceText(e.sidebar.Draw(components.DrawContext{
		Max:    components.Size{Width: sidebar.Width, Height: 32},
		Method: xui.WidthUnicode,
	}))
}

func TestEditorPlanFocusOpensModelPickerAndReturnsTypingToComposer(t *testing.T) {
	e := newTestEditor(t)
	e.App = app.NewApp(nil)
	e.sidebar.SetPlan(session.Plan{Revision: 1, Items: []session.PlanItem{{
		ID: "step-1", Content: "change the code", Status: session.PlanInProgress, Type: session.StepEdit,
	}}})
	e.sidebar.ConfigureModels([]string{"picker-model"})
	_ = e.Draw(components.DrawContext{
		Max: components.Size{Width: 120, Height: 30}, Method: xui.WidthUnicode,
	})
	e.Focus(&e.composer.Chat)
	assert.Equal(t, &e.composer.Chat, e.App.Focused())
	dispatchKey := func(key xui.KeyEvent) *components.EventContext {
		ctx := &components.EventContext{}
		if focused := e.App.Focused(); focused != nil && focused != e {
			focused.Handle(ctx, key)
		}
		if !ctx.Consume {
			e.Handle(ctx, key)
		}
		return ctx
	}

	altP := dispatchKey(xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'p', Mods: xui.ModAlt})
	require.True(t, altP.Consume && altP.Redraw)
	require.True(t, e.sidebar.PlanFocused())
	assert.Same(t, e, e.App.Focused(), "the editor root must see model-picker keys before ChatInput")

	dispatchKey(xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'm'})
	assert.Contains(t, sidebarText(e), "picker-model", "m opens the selected step's model picker")

	dispatchKey(xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'x'})
	assert.False(t, e.sidebar.PlanFocused(), "typing closes the sidebar keyboard mode")
	assert.Equal(t, &e.composer.Chat, e.App.Focused(), "typing restores real application focus to ChatInput")
	assert.Equal(t, "x", e.composer.Chat.Value, "the releasing rune reaches the composer exactly once")
}

// TestEditorComposerFocusReleasesPlanKeys pins the routing contract behind
// “the sidebar steals control keys while I type”: while ChatInput holds real
// focus, the plan pane owns no keys — anything the composer passes up falls
// through, never to the plan pane.
func TestEditorComposerFocusReleasesPlanKeys(t *testing.T) {
	e := newTestEditor(t)
	e.App = app.NewApp(nil)
	e.sidebar.SetPlan(session.Plan{Revision: 1, Items: []session.PlanItem{{
		ID: "step-1", Content: "change the code", Status: session.PlanInProgress, Type: session.StepEdit,
	}}})
	_ = e.Draw(components.DrawContext{
		Max: components.Size{Width: 120, Height: 30}, Method: xui.WidthUnicode,
	})
	e.Focus(&e.composer.Chat)
	require.Equal(t, &e.composer.Chat, e.App.Focused())

	dispatch := func(key xui.KeyEvent) *components.EventContext {
		ctx := &components.EventContext{}
		if focused := e.App.Focused(); focused != nil && focused != e {
			focused.Handle(ctx, key)
		}
		if !ctx.Consume {
			e.Handle(ctx, key)
		}
		return ctx
	}

	// alt+P focuses the plan pane's keyboard mode and moves real focus to the
	// editor root; clicking back into the composer moves real focus again —
	// the leak is that the plan pane keeps its keyboard mode.
	altP := dispatch(xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'p', Mods: xui.ModAlt})
	require.True(t, altP.Consume)
	require.True(t, e.sidebar.PlanFocused())
	e.Focus(&e.composer.Chat)

	escape := dispatch(xui.KeyEvent{Press: true, Code: xui.KeyEscape})
	assert.False(t, escape.Consume, "the plan pane must not consume keys the composer passes up")
	assert.False(t, e.sidebar.PlanFocused(), "composer focus releases the plan keyboard mode")

	typed := dispatch(xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'm'})
	assert.True(t, typed.Consume, "the composer consumes typing")
	assert.Equal(t, "m", e.composer.Chat.Value, "'m' types into the composer, not the picker")
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
func TestEditorCtrlATogglesApprovalToast(t *testing.T) {
	e := newTestEditor(t)

	ctx := &components.EventContext{}
	e.Handle(ctx, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'a', Mods: xui.ModCtrl})
	require.True(t, ctx.Consume && ctx.Redraw)
	assert.True(t, e.sidebar.Approved())
	assert.Equal(t, "Plan approved", e.toast.Message)

	ctx = &components.EventContext{}
	e.Handle(ctx, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'a', Mods: xui.ModCtrl})
	require.False(t, e.sidebar.Approved())
	assert.Equal(t, "Plan stopped", e.toast.Message)
}

func TestEditorSidebarFollowsUsageAndClear(t *testing.T) {
	e := newTestEditor(t)

	e.transcript.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
		ID:    "m1",
		State: session.StateComplete,
		Usage: session.TokenUsage{PromptTokens: 500, TotalTokens: 600},
	}})
	e.transcript.Sync()

	assert.Contains(t, sidebarText(e), "in 500", "per-turn usage reaches the panel")
	assert.Contains(t, sidebarText(e), "50%", "context fill is derived from the window")

	e.ClearSession()
	assert.Contains(t, sidebarText(e), "awaiting usage", "/clear resets the panel")
}

// TestEditorSettingsCarrySkills: the settings pane's plan tab lists the
// session's skills from a one-time discovery — later reads hit the cache,
// never the directory again, and the sidebar carries no skills at all.
func TestEditorSettingsCarrySkills(t *testing.T) {
	e := newTestEditor(t)
	root := t.TempDir()
	dir := filepath.Join(root, "grep-me")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"),
		[]byte("---\nname: grep-me\ndescription: finds things\n---\n"), 0o644))
	e.skillPath = root
	e.discoveredSkills = nil
	e.skillsResolved = false

	names := e.skillNames()
	assert.True(t, e.skillsResolved, "the settings wiring resolves skills once")
	assert.Contains(t, names, "grep-me", "discovered skills reach the plan settings tab")

	require.NoError(t, os.RemoveAll(dir))
	assert.Contains(t, e.skillNames(), "grep-me", "the cache outlives the directory")
	assert.NotContains(t, sidebarText(e), "grep-me", "the sidebar status tab carries no skills")
}

// TestEditorComposerHeightUsesContentWidth: the composer height must be
// measured at the width it is actually drawn at (contentW, after the sidebar
// takes its columns). Measuring at the full terminal width under-grows the
// composer and scrolls the first line out of view.
func TestEditorComposerHeightUsesContentWidth(t *testing.T) {
	e := newTestEditor(t)
	require.True(t, e.sidebar.Visible())

	const total = 120
	contentW := total - e.sidebar.ReserveWidth(total)
	e.composer.Chat.Value = strings.Repeat("w", 90)
	e.composer.Chat.Cursor = len(e.composer.Chat.Value)

	root := e.Draw(components.DrawContext{
		Max:    components.Size{Width: total, Height: 30},
		Method: xui.WidthUnicode,
	})
	chatSurf := root.Children[1].Surface
	want := e.composer.PreferredHeight(contentW, xui.WidthUnicode)
	require.GreaterOrEqual(t, chatSurf.Size.Height, want,
		"composer must be granted the height its wrapped content needs at content width")
}

// TestEditorOverlayHeightUsesContentWidth: the same for the ask overlay, which
// takes the composer's slot. Measured at the full terminal width it under-counts
// its wrapped rows, and the ask loses its last options off the bottom.
func TestEditorOverlayHeightUsesContentWidth(t *testing.T) {
	e := newTestEditor(t)
	require.True(t, e.sidebar.Visible())

	const total = 120
	contentW := total - e.sidebar.ReserveWidth(total)
	require.Less(t, contentW, total, "the sidebar must take columns for this to prove anything")

	e.overlays.Apply(controller.PermissionAskMsg{
		Request: permission.Request{
			Tool:    "bash",
			Action:  permission.ActionBash,
			Command: strings.Repeat("curl https://example.invalid/a/rather/long/path ", 4),
		},
		Reply: make(chan controller.AskReply, 1),
	})

	root := e.Draw(components.DrawContext{
		Max:    components.Size{Width: total, Height: 40},
		Method: xui.WidthUnicode,
	})
	askSurf := root.Children[1].Surface
	want, overlay := e.overlays.PreferredBottomHeight(contentW, xui.WidthUnicode)
	require.True(t, overlay, "the ask owns the bottom slot")
	require.GreaterOrEqual(t, askSurf.Size.Height, want,
		"the ask must be granted the height its wrapped body needs at content width")
}
