package app

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/chat"
	"github.com/alvnukov/cozyphi/internal/tui/sidebar"
)

// sidebarHost mounts the sidebar the way the session shell does: as a child
// surface of a host widget, so App's hit test must descend one level to
// reach it — exactly the delivery path a real chip click takes.
type sidebarHost struct {
	sb   *sidebar.Sidebar
	surf components.Surface
}

func (*sidebarHost) Handle(*components.EventContext, xui.Event) {}

func (h *sidebarHost) Draw(components.DrawContext) components.Surface { return h.surf }

// repaint draws one fresh sidebar frame into the host surface. Draw records
// the sidebar's hit zones as a side effect, so this is also how the test
// moves the painted affordances and their zones forward together.
func (h *sidebarHost) repaint() {
	sbSurf := h.sb.Draw(components.DrawContext{
		Max:    components.Size{Width: sidebar.Width, Height: 24},
		Method: xui.WidthUnicode,
	})
	h.surf = components.NewSurface(80, 24, h)
	h.surf.Children = []components.SubSurface{{Surface: sbSurf}}
}

// findAll returns the (x, y) of every row whose text contains needle, top to
// bottom. Coordinates are sidebar-local, which equals screen coordinates
// here because the child surface sits at the origin.
func (h *sidebarHost) findAll(needle string) [][2]int {
	var out [][2]int
	for y, line := range strings.Split(components.SurfaceText(h.surf.Children[0].Surface), "\n") {
		if strings.Contains(line, needle) {
			out = append(out, [2]int{strings.Index(line, needle), y})
		}
	}
	return out
}

// press feeds a left press through the App's real delivery path: capture,
// frame hit test, focus arbitration, local-coordinate delivery to the
// sidebar, and bubbling for whatever it refuses.
func press(t *testing.T, a *App, x, y int) {
	t.Helper()
	a.handleEvent(xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, X: x, Y: y})
}

// TestSidebarChipClicksDeliverThroughAppDispatch guards the reopened bug:
// settings values could be seen but not changed. Plain digits used to die in
// the focused composer; the steppers are mouse-driven, so the contract is
// that a chip click travels the full App dispatch — frame hit test with the
// composer still holding keyboard focus — and reaches the controller
// callback, no keyboard entry anywhere.
func TestSidebarChipClicksDeliverThroughAppDispatch(t *testing.T) {
	sb := sidebar.NewSidebar(components.DefaultTheme(), 0)
	sb.Toggle()

	var gotMain []int
	sb.ConfigureContext(150_000, 100_000, func(tokens int) {
		gotMain = append(gotMain, tokens)
	}, func(int) {})

	host := &sidebarHost{sb: sb}
	host.repaint()

	a := NewApp(nil)
	a.SetRoot(host)
	composer := &chat.ChatInput{}
	a.focused = composer
	a.lastSurf = host.surf

	// Switch to the settings tab by clicking its tab label.
	tabs := host.findAll("settings")
	require.NotEmpty(t, tabs, "the settings tab label must be painted")
	press(t, a, tabs[0][0]+2, tabs[0][1])
	host.repaint()
	a.lastSurf = host.surf

	chips := host.findAll("⊕")
	require.Len(t, chips, 2, "one plus chip per context row")
	// The chips hug the value: the row paints `compact ⊖ 150k ⊕` from the
	// content edge (x=2) — label(7) gap ⊖ gap 150k gap ⊕ — so the ⊕ cell
	// lands at 2+7+2+4+2 = 17.
	plusCol := 17

	// The compact row's + steps the reminder threshold by 10k while the
	// composer keeps keyboard focus — clicks must not need focus to land.
	press(t, a, plusCol, chips[0][1])
	require.Equal(t, []int{160_000}, gotMain)
	require.Same(t, composer, a.focused, "the composer keeps the keyboard")

	// The display setter is the view's push-back; with it in place another
	// click keeps stepping instead of repeating the same value.
	sb.SetReminderThreshold(160_000)
	host.repaint()
	a.lastSurf = host.surf
	press(t, a, plusCol, chips[0][1])
	require.Equal(t, []int{160_000, 170_000}, gotMain)
}
