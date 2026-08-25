package toast

import (
	"strings"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

func TestToastDrawSuccess(t *testing.T) {
	toast := Toast{Theme: components.DefaultTheme()}
	toast.Show("Selection copied to clipboard", ToastSuccess, time.Second)
	s := toast.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 24}, Method: xui.WidthUnicode})
	if len(s.Children) != 1 {
		t.Fatalf("children=%d", len(s.Children))
	}
	panel := s.Children[0].Surface
	var row strings.Builder
	for x := 0; x < panel.Size.Width; x++ {
		ch := panel.Buffer[panel.Size.Width+x].Char // y=1 content row
		if ch == "" {
			ch = " "
		}
		row.WriteString(ch)
	}
	got := row.String()
	if !strings.Contains(got, "Selection copied") {
		t.Fatalf("toast row=%q", got)
	}
	if !strings.Contains(got, "✓") {
		t.Fatalf("missing checkmark: %q", got)
	}
}
