package footer

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

// The footer is where a resumed session's id is visible from the first frame.
// Once streaming starts, the transcript owns the live model/thinking status;
// the footer keeps the session id but drops the duplicate generating spinner.
func TestFooterShowsSessionID(t *testing.T) {
	f := NewFooterChrome(components.DefaultTheme(), 0)
	f.SetSessionID(func() string { return "abcdef1234567890" })

	draw := func() string {
		return components.SurfaceText(f.Draw(components.DrawContext{
			Max:    components.Size{Width: 60, Height: 1},
			Method: xui.WidthUnicode,
		}, 60))
	}

	assert.Contains(t, draw(), "abcdef12")

	f.Activity().Apply(controller.ActivityStreaming)
	busy := draw()
	assert.NotContains(t, busy, "Generating…")
	assert.Contains(t, busy, "abcdef12")
	assert.False(t, f.Activity().ShowFooterSpinner(), "streaming activity lives in the transcript")
}

func TestJoinBorderParts(t *testing.T) {
	if got := joinBorderParts("↑1.2k ↓800 Σ2.0k", "4%/128k"); got != "↑1.2k ↓800 Σ2.0k 4%/128k" {
		t.Fatalf("got %q", got)
	}
	if got := joinBorderParts("", "4%/128k"); got != "4%/128k" {
		t.Fatalf("got %q", got)
	}
	if got := joinBorderParts("↑1.2k", ""); got != "↑1.2k" {
		t.Fatalf("got %q", got)
	}
	if got := joinBorderParts("", ""); got != "" {
		t.Fatalf("got %q", got)
	}
}

// RunEndedMsg drops run-derived footer activity when the pipeline goes idle;
// outcome labels the user still reads (Stopped) must survive it.
func TestFooterRunEndedResetsRunActivity(t *testing.T) {
	f := NewFooterChrome(components.DefaultTheme(), 0)

	f.Apply(controller.SetActivityMsg{Activity: controller.ActivityStreaming})
	f.Apply(controller.RunEndedMsg{})
	assert.Equal(t, controller.ActivityIdle, f.Activity().Current)

	f.Apply(controller.SetActivityMsg{Activity: controller.ActivityCancelled})
	f.Apply(controller.RunEndedMsg{})
	assert.Equal(t, controller.ActivityCancelled, f.Activity().Current)
}
