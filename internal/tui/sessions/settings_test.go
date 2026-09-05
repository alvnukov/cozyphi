package sessions

import (
	"context"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/tasks"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

type editorSettingsStore struct {
	snapshot harnesssettings.Snapshot
}

func (s *editorSettingsStore) Snapshot() harnesssettings.Snapshot { return s.snapshot }

func (s *editorSettingsStore) Apply(
	_ context.Context,
	draft harnesssettings.Draft,
) (harnesssettings.Snapshot, error) {
	s.snapshot.Plan = draft.Plan
	return s.snapshot, nil
}

func TestEditorSettingsModalOwnsInputAndDrawsAboveComposer(t *testing.T) {
	e := newEditorWithSettings(t)
	e.composer.Chat.Value = "composer draft"

	e.ShowSettings()
	require.True(t, e.settings.Visible())
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'x'})
	assert.Equal(t, "composer draft", e.composer.Chat.Value)

	root := e.Draw(components.DrawContext{
		Max: components.Size{Width: 100, Height: 30}, Method: xui.WidthUnicode,
	})
	require.True(t, childContains(root, "Harness settings"))

	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
	assert.False(t, e.settings.Visible())
}

func TestEditorCtrlCommaOpensSettingsModal(t *testing.T) {
	e := newEditorWithSettings(t)

	e.Handle(&components.EventContext{}, xui.KeyEvent{
		Press: true, Code: xui.KeyRune, Rune: ',', Mods: xui.ModCtrl,
	})

	assert.True(t, e.settings.Visible())
}

func newEditorWithSettings(t *testing.T) *View {
	t.Helper()
	home, cwd := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())
	bus := controller.NewBus(nil)
	ctrl, err := controller.NewController(bus, proj, cwd, "")
	require.NoError(t, err)
	t.Cleanup(ctrl.Close)
	store := &editorSettingsStore{snapshot: harnesssettings.Snapshot{
		Path: proj.Global().ConfigFile(), Plan: plangate.DefaultDefaults(),
	}}
	return NewView(
		nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "m", "", 1000, nil, nil, store,
	)
}

func childContains(root components.Surface, text string) bool {
	for _, child := range root.Children {
		if strings.Contains(components.SurfaceText(child.Surface), text) {
			return true
		}
	}
	return false
}

func TestViewsShareOneSettingsManager(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())
	runtime, err := plangate.NewRuntime(plangate.DefaultDefaults())
	require.NoError(t, err)
	manager, err := harnesssettings.Open(proj.Global().ConfigFile(), runtime, nil)
	require.NoError(t, err)
	newView := func() *View {
		bus := controller.NewBus(nil)
		ctrl, err := controller.NewController(bus, proj, cwd, "")
		require.NoError(t, err)
		t.Cleanup(ctrl.Close)
		return NewView(nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "m", "", 1000, nil, nil, manager)
	}
	first, second := newView(), newView()
	require.Equal(t, tasks.AccessWrite, second.ctrl.TasksAccess())

	// A commit made from the first session's pane reaches the second session.
	first.settings.Show()
	draft := manager.Snapshot().Draft()
	draft.Tasks = tasks.AccessRead
	_, err = manager.Apply(t.Context(), draft)
	require.NoError(t, err)
	require.Equal(t, tasks.AccessRead, first.ctrl.TasksAccess())
	require.Equal(t, tasks.AccessRead, second.ctrl.TasksAccess())

	// A closed session detaches: the commit no longer touches its controller.
	require.NoError(t, second.Close(t.Context()))
	next := manager.Snapshot().Draft()
	next.Tasks = tasks.AccessAsk
	_, err = manager.Apply(t.Context(), next)
	require.NoError(t, err)
	require.Equal(t, tasks.AccessAsk, first.ctrl.TasksAccess())
	require.Equal(t, tasks.AccessRead, second.ctrl.TasksAccess())
}
