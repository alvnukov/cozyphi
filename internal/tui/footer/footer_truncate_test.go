package footer

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

func drawRow(f *FooterChrome, width int) string {
	return components.SurfaceText(f.Draw(components.DrawContext{
		Max:    components.Size{Width: width, Height: 1},
		Method: xui.WidthUnicode,
	}, width))
}

// A long status must be cut with an ellipsis and stop short of the
// right-aligned update hint — not silently clipped under it.
func TestFooterEllipsizesStatusBeforeHint(t *testing.T) {
	f := NewFooterChrome(components.DefaultTheme(), 0)
	f.SetHookStatus(strings.Repeat("hook ", 30))
	f.Apply(controller.UpdateAvailableMsg{Latest: "v9.9.9"})

	row := drawRow(f, 60)
	assert.Contains(t, row, "hook hook hook hook hook…")
	assert.Contains(t, row, "9.9.9 available · cozyphi update")
}

func TestFooterEllipsizesStatusWithoutHint(t *testing.T) {
	f := NewFooterChrome(components.DefaultTheme(), 0)
	f.SetHookStatus(strings.Repeat("x", 100))

	row := drawRow(f, 40)
	assert.Contains(t, row, "…")
	assert.NotContains(t, row, strings.Repeat("x", 38))
}
