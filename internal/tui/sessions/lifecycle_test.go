package sessions

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/editmode"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

func TestViewConstructionIsInactive(t *testing.T) {
	selected := newTestEditor(t)
	selected.App = app.NewApp(nil)
	selected.Focus(&selected.composer.Chat)
	require.NoError(t, selected.applyEditingMode(editmode.Readline))
	before := keys.Label(keys.CmdPalette)
	hidden := NewView(selected.App, controller.NewBus(nil), selected.ctrl, nil, nil,
		components.DefaultTheme(), selected.cwd, "m", "", 1000, nil, nil)
	require.False(t, hidden.Active())
	require.Same(t, &selected.composer.Chat, selected.App.Focused())
	require.Equal(t, before, keys.Label(keys.CmdPalette))
	hidden.FocusEditor()
	require.Same(t, &selected.composer.Chat, selected.App.Focused())
}

func TestViewBackgroundAskRetainsDraftPaletteAndFocus(t *testing.T) {
	first, second := newTestEditor(t), newTestEditor(t)
	application := app.NewApp(nil)
	first.App, second.App = application, application
	first.composer.Chat.Value = "first draft"
	second.composer.Chat.Value = "second draft"
	first.PushSubmenu("Saved", []palette.PaletteCommand{{ID: "saved", Verb: "saved"}})
	first.SetActive(false)
	second.Focus(&second.composer.Chat)
	first.Publish(controller.PermissionAskMsg{})
	first.Publish(controller.SetActivityMsg{Activity: controller.ActivityTools})
	second.DrainNow()
	require.Empty(t, first.Status().Waiting, "a sibling must not drain this bus")
	first.DrainNow()
	require.Equal(t, "permission", first.Status().Waiting)
	require.True(t, first.Status().Running)
	_, visible := first.composer.PaletteOverlay(components.DrawContext{})
	require.True(t, visible)
	require.Same(t, &second.composer.Chat, application.Focused())
	require.False(t, second.overlays.Active())
	require.Empty(t, second.Status().Waiting)
	second.SetActive(false)
	first.SetActive(true)
	require.Same(t, first, application.Focused())
	_, visible = first.composer.PaletteOverlay(components.DrawContext{})
	require.False(t, visible, "pending ask presents on selection")
	first.Update(controller.PermissionDismissMsg{})
	require.Empty(t, first.Status().Waiting)
	require.Same(t, &first.composer.Chat, application.Focused())
	require.Equal(t, "first draft", first.composer.Chat.Value)
	require.Equal(t, "second draft", second.composer.Chat.Value)
	first.SetActive(false)
	first.Update(controller.RunEndedMsg{})
	require.Equal(t, 1, first.Status().Unread)
	require.False(t, first.Status().Running)
	first.SetActive(true)
	require.Equal(t, 1, first.Status().Unread, "activation alone does not view the transcript")
}

func TestViewSelectionRestoresLogicalFocusAndProfile(t *testing.T) {
	first, second := newTestEditor(t), newTestEditor(t)
	application := app.NewApp(nil)
	first.App, second.App = application, application
	require.NoError(t, first.applyEditingMode(editmode.Readline))
	first.PushSubmenu("Saved", []palette.PaletteCommand{{ID: "saved", Verb: "saved"}})
	saved := application.Focused()
	first.SetActive(false)
	require.NoError(t, second.applyEditingMode(editmode.Standard))
	standard := keys.Label(keys.CmdPalette)
	require.NoError(t, first.applyEditingMode(editmode.Vim))
	require.Equal(t, standard, keys.Label(keys.CmdPalette), "background preferences cannot install global keys")
	second.SetActive(false)
	first.SetActive(true)
	require.Same(t, saved, application.Focused())
	_, visible := first.composer.PaletteOverlay(components.DrawContext{})
	require.True(t, visible)
	require.Equal(t, editmode.Vim, first.composer.Chat.EditingMode())
	selectedLabel := keys.Label(keys.CmdPalette)
	require.NoError(t, keys.SetProfile(editmode.Vim))
	require.Equal(t, keys.Label(keys.CmdPalette), selectedLabel)
	first.Publish(controller.SessionEventMsg{Event: session.AssistantMessageUpdate{Message: session.Message{
		ID: "error", State: session.StateError,
	}}})
	first.DrainNow()
	require.Equal(t, "Run failed", first.Status().Error)
	require.Empty(t, second.Status().Error)
}

func TestViewCloseStopsBranchAndLocalShell(t *testing.T) {
	e := newTestEditor(t)
	require.NoError(t, os.Mkdir(filepath.Join(e.cwd, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(e.cwd, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o600))
	e.StartBranchWatch()
	done := e.lifetime.branchDone
	e.StartBranchWatch()
	require.Equal(t, done, e.lifetime.branchDone, "one watcher per retained view")
	e.Update(controller.SubmitMsg{Text: "!printf accepted; sleep 30"})
	require.True(t, e.Status().Running)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, e.Close(ctx))
	require.NoError(t, e.Close(ctx))
	require.False(t, e.Active())
	require.False(t, e.bashRunner.Running())
	require.ErrorIs(t, e.lifetime.ctx.Err(), context.Canceled)
	select {
	case <-done:
	default:
		t.Fatal("branch watcher survived Close")
	}
	e.DrainNow()
	require.NotEmpty(t, e.transcript.Snapshot().Messages, "accepted shell completion remains on the view bus")
	e.SetActive(true)
	require.False(t, e.Active(), "closed views cannot be reactivated")
	e.Update(controller.SubmitMsg{Text: "!touch after-close"})
	_, err := os.Stat(filepath.Join(e.cwd, "after-close"))
	require.True(t, os.IsNotExist(err))
}

func TestViewBackgroundDismissKeepsSavedPalette(t *testing.T) {
	e := newTestEditor(t)
	e.App = app.NewApp(nil)
	e.PushSubmenu("Saved", []palette.PaletteCommand{{ID: "saved", Verb: "saved"}})
	saved := e.App.Focused()
	e.SetActive(false)
	e.Update(controller.ContinueAskMsg{})
	e.Update(controller.ContinueDismissMsg{})
	require.Empty(t, e.Status().Waiting)
	e.SetActive(true)
	require.Same(t, saved, e.App.Focused())
	_, visible := e.composer.PaletteOverlay(components.DrawContext{})
	require.True(t, visible)
}
