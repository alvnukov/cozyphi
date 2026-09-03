package editor

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

// TestEditorAskOwnsTheMouse: while a modal ask is up, a click anywhere
// else dies at the modal, and a click on an option row answers the ask —
// the two halves of the mouse-modality contract.
func TestEditorAskOwnsTheMouse(t *testing.T) {
	e := newTestEditor(t)
	require.True(t, e.sidebar.Visible())

	reply := make(chan controller.AskReply, 1)
	e.overlays.Apply(controller.PermissionAskMsg{
		Request: permission.Request{Tool: "bash", Action: permission.ActionBash, Command: "curl https://x"},
		Reply:   reply,
	})
	root := e.Draw(components.DrawContext{
		Max:    components.Size{Width: 120, Height: 40},
		Method: xui.WidthUnicode,
	})

	// A press over the sidebar must not reach it.
	ctx := &components.EventContext{}
	e.Handle(ctx, xui.MouseEvent{X: 110, Y: 5, Button: xui.MouseLeft, Action: xui.MousePress})
	require.True(t, ctx.Consume, "the modal must consume the sidebar click")
	require.True(t, e.overlays.PermissionActive(), "the ask must survive a stray click")

	// A click on the selected option resolves the permission.
	askChild := root.Children[1]
	rows := strings.Split(components.SurfaceText(askChild.Surface), "\n")
	y := -1
	for i, row := range rows {
		if strings.Contains(row, "Approve") {
			y = askChild.Origin.Y + i
			break
		}
	}
	require.NotEqual(t, -1, y, "the panel must render the Approve option")
	e.Handle(&components.EventContext{}, xui.MouseEvent{X: 4, Y: y, Button: xui.MouseLeft, Action: xui.MousePress})
	require.False(t, e.overlays.PermissionActive(), "clicking the selected option must resolve the ask")

	select {
	case r := <-reply:
		require.True(t, r.Approved)
	default:
		t.Fatal("expected a reply")
	}
}
