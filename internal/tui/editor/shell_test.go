package editor_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/clipboard"
	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/editor"
	"github.com/alvnukov/cozyphi/internal/tui/sessions"
)

func TestShellRetainsDraftsAndDrainsBackgroundAsk(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	application := app.NewApp(nil)
	registry := sessions.NewRegistry(12, nil)
	busA, busB := controller.NewBus(nil), controller.NewBus(nil)
	makeView := func(bus *controller.Bus) *sessions.View {
		view := sessions.NewView(application, bus, nil, nil, nil, components.DefaultTheme(),
			t.TempDir(), "test", "", 1000, nil, nil)
		view.SetClipboardReader(func() (clipboard.Image, bool, error) { return clipboard.Image{}, false, nil })
		return view
	}
	a, b := makeView(busA), makeView(busB)
	idA, err := registry.Open("first", a)
	require.NoError(t, err)
	idB, err := registry.Open("second", b)
	require.NoError(t, err)
	shell := editor.NewEditor(application, registry)
	t.Cleanup(func() { require.NoError(t, shell.Close(context.WithoutCancel(t.Context()))) })
	// Draft isolation does not depend on the host clipboard (which may
	// contain an image and legitimately intercept a synthetic paste event).
	typeDraft := func(text string) {
		for _, ch := range text {
			shell.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyRune, Rune: ch, Press: true})
		}
	}
	typeDraft("draftAlpha")
	require.NoError(t, shell.Activate(idB))
	typeDraft("draftBeta")
	require.Contains(t, drawText(shell), "draftBeta")
	require.NotContains(t, drawText(shell), "draftAlpha")

	focus := application.Focused()
	reply := make(chan controller.AskReply, 1)
	busA.Publish(controller.PermissionAskMsg{Reason: "background approval", Reply: reply})
	shell.DrainNow()
	require.Same(t, focus, application.Focused(), "background request cannot take focus")
	require.Equal(t, "permission", a.Status().Waiting)
	require.Empty(t, b.Status().Waiting)
	select {
	case <-reply:
		t.Fatal("switching/draining must not answer an approval")
	default:
	}

	// Capture reaches navigation even though the selected View has a modal.
	require.NoError(t, shell.Activate(idA))
	ctx := &components.EventContext{}
	shell.Capture(ctx, xui.KeyEvent{Code: xui.KeyF10, Mods: xui.ModCtrl, Press: true})
	require.True(t, ctx.Consume)
	active, ok := registry.Active()
	require.True(t, ok)
	require.Equal(t, idB, active.ID)
	require.Contains(t, drawText(shell), "draftBeta")
	busA.Publish(controller.PermissionDismissMsg{})
	shell.DrainNow()
	require.NoError(t, shell.Activate(idA))
	require.Contains(t, drawText(shell), "draftAlpha")
	require.NotContains(t, drawText(shell), "draftBeta")
}

func drawText(shell *editor.Editor) string {
	surface := shell.Draw(
		components.DrawContext{Max: components.Size{Width: 100, Height: 30}, Method: xui.WidthUnicode},
	)
	var text strings.Builder
	var visit func(components.Surface)
	visit = func(s components.Surface) {
		for _, cell := range s.Buffer {
			text.WriteString(cell.Char)
		}
		for _, child := range s.Children {
			visit(child.Surface)
		}
	}
	visit(surface)
	return text.String()
}

func TestShellRefusesExitWhileBackgroundSessionRuns(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	application := app.NewApp(nil)
	registry := sessions.NewRegistry(12, nil)
	busA, busB := controller.NewBus(nil), controller.NewBus(nil)
	makeView := func(bus *controller.Bus) *sessions.View {
		return sessions.NewView(application, bus, nil, nil, nil, components.DefaultTheme(),
			t.TempDir(), "test", "", 1000, nil, nil)
	}
	a, b := makeView(busA), makeView(busB)
	_, err := registry.Open("first", a)
	require.NoError(t, err)
	_, err = registry.Open("worker", b)
	require.NoError(t, err)
	shell := editor.NewEditor(application, registry)
	t.Cleanup(func() { require.NoError(t, shell.Close(context.WithoutCancel(t.Context()))) })

	busB.Publish(controller.SetActivityMsg{Activity: controller.ActivityTools})
	shell.DrainNow()
	require.True(t, b.Status().Running)
	require.True(t, shell.AcceptInterrupt(), "the first press arms the exit")
	require.True(t, shell.AcceptInterrupt(), "a running background session blocks the exit")
	text := drawText(shell)
	require.Contains(t, text, "2 worker")
	require.NotContains(t, text, "again to exit", "the refused exit withdraws its promise")

	busB.Publish(controller.RunEndedMsg{})
	shell.DrainNow()
	require.False(t, b.Status().Running)
	require.True(t, shell.AcceptInterrupt(), "a refused exit arms again")
	require.False(t, shell.AcceptInterrupt(), "with nothing running the armed exit proceeds")
}

func TestShellCloseReportsFinishedViewsUnderExpiredContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	application := app.NewApp(nil)
	registry := sessions.NewRegistry(12, nil)
	for _, name := range []string{"first", "second"} {
		view := sessions.NewView(application, controller.NewBus(nil), nil, nil, nil, components.DefaultTheme(),
			t.TempDir(), "test", "", 1000, nil, nil)
		_, err := registry.Open(name, view)
		require.NoError(t, err)
	}
	shell := editor.NewEditor(application, registry)
	require.NoError(t, shell.Close(context.WithoutCancel(t.Context())))
	expired, cancel := context.WithCancel(t.Context())
	cancel()
	for range 50 {
		require.NoError(t, shell.Close(expired), "finished cleanup never reports the caller's deadline")
	}
}
