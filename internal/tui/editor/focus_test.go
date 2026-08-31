package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/components/chat"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/ctxpane"
	"github.com/alvnukov/cozyphi/internal/tui/helppane"
	"github.com/alvnukov/cozyphi/internal/tui/overlays"
)

// While a modal overlay owns the keyboard, focus requests aimed at composer
// widgets must land on the editor root, where the overlay intercepts keys;
// once the overlay is gone the request reaches its target again.
func TestFocusStaysAtRootWhileOverlayActive(t *testing.T) {
	o := overlays.NewOverlays(components.DefaultTheme(), nil, nil, nil, nil)
	o.Apply(controller.PermissionAskMsg{})
	e := &Editor{App: app.NewApp(nil), overlays: o}

	target := &chat.ChatInput{}
	e.Focus(target)
	assert.Same(t, e, e.App.Focused())

	o.Apply(controller.PermissionDismissMsg{})
	e.Focus(target)
	assert.Same(t, target, e.App.Focused())
}

// While the context browser covers the screen it owns the keyboard: focus
// requests aimed at composer widgets land on the editor root, where
// HandleEvent intercepts keys before the composer can eat them.
func TestFocusStaysAtRootWhileContextBrowserVisible(t *testing.T) {
	pane := ctxpane.New(
		components.DefaultTheme(),
		func() agent.ContextView { return agent.ContextView{} },
		nil, nil, nil, nil,
	)
	pane.Show()
	e := &Editor{
		App:      app.NewApp(nil),
		overlays: overlays.NewOverlays(components.DefaultTheme(), nil, nil, nil, nil),
		ctxpane:  pane,
	}

	target := &chat.ChatInput{}
	e.Focus(target)
	assert.Same(t, e, e.App.Focused())
}

// Opening the browser must actively take focus from the composer: app.dispatch
// sends key events to the focused widget first, and the chat input swallows
// arrows and letters before the editor ever sees them.
func TestShowContextGrabsFocus(t *testing.T) {
	pane := ctxpane.New(
		components.DefaultTheme(),
		func() agent.ContextView { return agent.ContextView{} },
		nil, nil, nil, nil,
	)
	e := &Editor{App: app.NewApp(nil), ctxpane: pane}

	e.ShowContext()
	assert.True(t, pane.Visible())
	assert.Same(t, e, e.App.Focused())
}

// The help screen scrolls with the same bare keys the composer types, so it
// has to own the keyboard the moment it opens.
func TestShowHelpGrabsFocus(t *testing.T) {
	pane := helppane.New(components.DefaultTheme(), nil)
	e := &Editor{App: app.NewApp(nil), help: pane}

	e.ShowHelp()
	assert.True(t, pane.Visible())
	assert.Same(t, e, e.App.Focused())
}
