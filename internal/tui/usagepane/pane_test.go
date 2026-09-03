package usagepane

import (
	"errors"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

func fixtureStats() controller.SessionStats {
	return controller.SessionStats{
		Model:         "glm-4.5-air",
		ProviderID:    "zai-coding-plan",
		ContextWindow: 128000,
		InputTokens:   12000,
		OutputTokens:  3400,
		CachedTokens:  8000,
		TotalTokens:   23400,
		Rounds:        7,
		StartedAt:     time.Now().Add(-90 * time.Minute),
		ContextTokens: 31000,
	}
}

func newTestPane() (*Pane, *int, *int) {
	refreshes := 0
	closes := 0
	p := New(
		components.DefaultTheme(),
		fixtureStats,
		func() { refreshes++ },
		func() { closes++ },
	)
	return p, &refreshes, &closes
}

func press(t *testing.T, p *Pane, code xui.KeyCode, r rune) bool {
	t.Helper()
	return p.HandleEvent(&components.EventContext{}, xui.KeyEvent{Press: true, Code: code, Rune: r})
}

func paneText(t *testing.T, p *Pane) string {
	t.Helper()
	return components.SurfaceText(p.Draw(components.DrawContext{Max: components.Size{Width: 64, Height: 24}}))
}

// TestShowFetchesAndRenders: opening the pane pulls the session snapshot,
// fires one quota fetch, and both sections render once the fetch lands.
func TestShowFetchesAndRenders(t *testing.T) {
	p, refreshes, _ := newTestPane()
	p.Show()

	require.True(t, p.Visible())
	require.Equal(t, 1, *refreshes, "Show kicks off exactly one fetch")
	assert.Contains(t, paneText(t, p), "fetching subscription usage…", "quota section starts loading")
	assert.Contains(t, paneText(t, p), "rounds 7", "session section renders immediately")

	p.Apply(controller.UsageQuotaMsg{
		ProviderID: "zai-coding-plan",
		Snapshot: provider.QuotaSnapshot{
			PlanName: "GLM Coding Plan",
			Limits: []provider.QuotaLimit{{
				Window: "5 hours", Used: 300000, Remaining: 700000, Total: 1000000,
				ResetsAt: time.Now().Add(2 * time.Hour),
			}},
		},
	})

	text := paneText(t, p)
	assert.Contains(t, text, "plan  GLM Coding Plan")
	assert.Contains(t, text, "5 hours")
	assert.Contains(t, text, "300k / 1.0M")
	assert.Contains(t, text, "resets in 2h0m0s")
	assert.Contains(t, text, "context 31k / 128k (24%)")
	assert.Contains(t, text, "████", "the bar has filled cells")
}

// TestEscClosesRFefreshes: Esc closes and fires onClose once; r re-pulls the
// session and starts another fetch without reopening.
func TestEscClosesRRefreshes(t *testing.T) {
	p, refreshes, closes := newTestPane()
	p.Show()

	require.True(t, press(t, p, xui.KeyEscape, 0))
	assert.False(t, p.Visible())
	assert.Equal(t, 1, *closes, "closing hands focus back exactly once")
	assert.False(t, press(t, p, xui.KeyRune, 'r'), "a hidden pane does not consume keys")

	p.Show()
	require.True(t, press(t, p, xui.KeyRune, 'r'))
	assert.Equal(t, 3, *refreshes, "two Shows plus one r")
	assert.Contains(t, paneText(t, p), "fetching subscription usage…", "refresh returns to loading")
}

// TestQuotaStates: unsupported providers, transport failures and stale
// messages each render their own line, without touching the session block.
func TestQuotaStates(t *testing.T) {
	p, _, _ := newTestPane()
	p.Show()

	p.Apply(controller.UsageQuotaMsg{ProviderID: "openai", Unsupported: true})
	assert.Contains(t, paneText(t, p), "openai has no subscription endpoint yet")
	assert.Contains(t, paneText(t, p), "rounds 7", "session section survives unsupported quota")

	p.Apply(controller.UsageQuotaMsg{ProviderID: "zai-coding-plan", Err: errors.New("dial tcp: connection refused")})
	assert.Contains(t, paneText(t, p), "connection refused")

	p.loading = true
	p.Apply(controller.UsageQuotaMsg{ProviderID: ""})
	assert.True(t, p.loading, "a fetch for a closed pane is dropped, loading stands")
}

// TestPaneConsumesEvents: while visible every key and mouse event is consumed
// so nothing leaks into the shell underneath.
func TestPaneConsumesEvents(t *testing.T) {
	p, _, _ := newTestPane()
	p.Show()

	assert.True(t, press(t, p, xui.KeyUp, 0))
	assert.True(t, press(t, p, xui.KeyRune, 'x'))
	assert.True(t, p.HandleEvent(&components.EventContext{}, xui.MouseEvent{Button: xui.MouseWheelDown}))
}
