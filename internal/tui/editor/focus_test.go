package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/app"
	"github.com/pulseaiclub/phi/internal/components/chat"
	"github.com/pulseaiclub/phi/internal/tui/controller"
	"github.com/pulseaiclub/phi/internal/tui/overlays"
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
