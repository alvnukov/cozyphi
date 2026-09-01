package footer

import (
	"strings"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

// liveSnap is a turn 42 seconds in: one finished assistant round that
// reported tokens and a second round still streaming.
func liveSnap() session.Snapshot {
	start := time.Now().Add(-42 * time.Second)
	return session.Snapshot{
		Messages: []session.Message{
			{ID: "u1", Role: session.RoleUser, Text: "go"},
			{
				ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
				Started: start, Ended: start.Add(30 * time.Second),
				Usage: session.TokenUsage{CompletionTokens: 1234},
			},
			{
				ID: "a2", Role: session.RoleAssistant, State: session.StateStreaming,
				Started: start.Add(30 * time.Second),
			},
		},
	}
}

// The streaming footer is the one consolidated activity line: phase verb,
// the turn's elapsed time, its token stream, and the interrupt hint — with
// the scan-bar spinner gone so the active transcript row keeps the only
// spinner glyph in view.
func TestLiveFooterNamesPhaseElapsedTokensAndInterrupt(t *testing.T) {
	f := NewFooterChrome(components.DefaultTheme(), 0)
	snap := liveSnap()
	f.SetLabelContext(func() session.Snapshot { return snap })
	f.Activity().Apply(controller.ActivityStreaming)

	row := drawRow(f, 80)
	assert.Contains(t, row, "✻")
	assert.Contains(t, row, "Generating…")
	assert.Contains(t, row, "42s")
	assert.Contains(t, row, "↓1.2k")
	assert.Contains(t, row, "Esc interrupts")
	assert.NotContains(t, row, "■", "the scan-bar spinner left the footer")
	assert.NotContains(t, row, "⬝", "the scan-bar spinner left the footer")
}

// The activity line dies with the turn: idle keeps the quiet status row
// without pulse glyph or interrupt hint.
func TestLiveFooterGoesOutWhenTheTurnEnds(t *testing.T) {
	f := NewFooterChrome(components.DefaultTheme(), 0)
	snap := liveSnap()
	f.SetLabelContext(func() session.Snapshot { return snap })
	f.Activity().Apply(controller.ActivityStreaming)
	assert.Contains(t, drawRow(f, 80), "Esc interrupts")

	f.Apply(controller.RunEndedMsg{})
	row := drawRow(f, 80)
	assert.NotContains(t, row, "Esc interrupts")
	assert.NotContains(t, row, "✻")
	assert.NotContains(t, row, "Generating")
}

// A narrow footer clips the activity spans with an ellipsis and still lands
// the right-aligned interrupt hint clear of them.
func TestLiveFooterClipsBeforeTheHint(t *testing.T) {
	f := NewFooterChrome(components.DefaultTheme(), 0)
	snap := liveSnap()
	f.SetLabelContext(func() session.Snapshot { return snap })
	f.SetModelSource(func() string { return strings.Repeat("verylongmodelname-", 4) })
	f.Activity().Apply(controller.ActivityStreaming)

	row := drawRow(f, 44)
	assert.Contains(t, row, "…")
	assert.Contains(t, row, "Esc interrupts")
}

func TestClipSpansCutsInsideASpan(t *testing.T) {
	spans := []components.Span{
		{Text: "abc"},
		{Text: "defgh"},
	}
	got := clipSpans(spans, 6, xui.WidthUnicode)
	text := ""
	for _, sp := range got {
		text += sp.Text
	}
	assert.Equal(t, "abcde…", text)
	assert.Equal(t, spans, clipSpans(spans, 8, xui.WidthUnicode), "a fitting run is untouched")
}
