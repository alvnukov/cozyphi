package editor_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/editor"
	"github.com/alvnukov/cozyphi/internal/tui/sessions"
)

func TestShellTitleForegroundBackgroundSwitchResumeClear(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	proj, err := project.Discover(t.TempDir())
	require.NoError(t, err)
	application := app.NewApp(nil)
	registry := sessions.NewRegistry(12, nil)
	makeView := func() (*sessions.View, *controller.Controller) {
		bus := controller.NewBus(nil)
		ctrl, createErr := controller.NewController(bus, proj, proj.Root(), "")
		require.NoError(t, createErr)
		view := sessions.NewView(application, bus, ctrl, nil, nil, components.DefaultTheme(),
			proj.Root(), "test", "", 1000, nil, nil)
		return view, ctrl
	}
	a, ca := makeView()
	b, _ := makeView()
	idA, err := registry.Open("parent", a)
	require.NoError(t, err)
	idB, err := registry.Open("child label", b)
	require.NoError(t, err)
	shell := editor.NewEditor(application, registry)
	t.Cleanup(func() { require.NoError(t, shell.Close(context.WithoutCancel(t.Context()))) })
	drawText(shell)
	require.Equal(t, session.ShortID(ca.SessionID()), shell.TerminalTitle())
	require.NoError(t, a.RenameSession("foreground title"))
	text := drawText(shell)
	require.Equal(t, "foreground title", shell.TerminalTitle())
	require.GreaterOrEqual(t, strings.Count(text, "foreground title"), 3, "selector, composer and footer agree")
	require.NoError(t, b.RenameSession("background title"))
	drawText(shell)
	require.Equal(t, "foreground title", shell.TerminalTitle())
	require.NoError(t, shell.Activate(idB))
	drawText(shell)
	require.Equal(t, "background title", shell.TerminalTitle())
	require.Equal(t, "child label", registry.Entries()[1].Name, "projection must not rename child registry labels")
	require.NoError(t, shell.Activate(idA))
	oldID := ca.SessionID()
	require.NoError(t, ca.Clear())
	drawText(shell)
	require.Equal(t, session.ShortID(ca.SessionID()), shell.TerminalTitle())
	require.NotEqual(t, oldID, ca.SessionID())
	_, err = ca.Resume(oldID)
	require.NoError(t, err)
	drawText(shell)
	require.Equal(t, "foreground title", shell.TerminalTitle())
	require.Error(t, a.RenameSession(strings.Repeat("界", 61)))
	require.Equal(t, "foreground title", shell.TerminalTitle())
}
