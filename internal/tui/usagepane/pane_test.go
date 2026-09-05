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

func TestOpenAIQuotaRendersPercentTokensAndResetLimitation(t *testing.T) {
	p, _, _ := newTestPane()
	p.Show()
	p.Apply(controller.UsageQuotaMsg{
		ProviderID: "openai",
		Snapshot: provider.QuotaSnapshot{
			PlanName: "plus",
			Limits: []provider.QuotaLimit{{
				Window: "5 hours", Unit: "percent", UsedPercent: 37,
				ResetsAt: time.Now().Add(5 * time.Hour),
			}},
			Tokens: []provider.QuotaTokenUsage{{Scope: "Codex profile lifetime", Tokens: 3500}},
			Reset: provider.QuotaResetSummary{
				Available: 2,
				Supported: true,
				Note:      "CozyPhi does not perform manual resets; use the official Codex UI.",
			},
		},
	})

	text := paneText(t, p)
	assert.Contains(t, text, "plan  plus")
	assert.Contains(t, text, "37% used · 63% remaining")
	assert.Contains(t, text, "████", "percent-only limits still fill the bar")
	assert.Contains(t, text, "tokens (Codex profile lifetime)  3.5k")
	assert.Contains(t, text, "manual resets  2 available")
	assert.Contains(t, text, "reset action: CozyPhi does not perform manual resets")
}

func TestOpenAITokenAvailability(t *testing.T) {
	for _, observed := range []bool{false, true} {
		p, _, _ := newTestPane()
		p.Show()
		snapshot := provider.QuotaSnapshot{Reset: provider.QuotaResetSummary{Supported: observed}}
		if observed {
			snapshot.Tokens = []provider.QuotaTokenUsage{{Scope: "Codex profile lifetime", Tokens: 0}}
		}
		p.Apply(controller.UsageQuotaMsg{ProviderID: "openai", Snapshot: snapshot})
		text := paneText(t, p)
		assert.Contains(t, text, "rate-limit data unavailable")
		if observed {
			assert.Contains(t, text, "tokens (Codex profile lifetime)  0")
			assert.Contains(t, text, "manual resets  0 available")
			assert.NotContains(t, text, "token data unavailable")
		} else {
			assert.Contains(t, text, "Codex profile token data unavailable")
			assert.NotContains(t, text, "manual resets  0")
		}
		assert.Contains(t, text, "rounds 7")
	}
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

func largeQuota() controller.UsageQuotaMsg {
	msg := controller.UsageQuotaMsg{ProviderID: "openai", Snapshot: provider.QuotaSnapshot{
		PlanName: "plus",
		Limits:   []provider.QuotaLimit{{Window: "5 hours", Unit: "percent", UsedPercent: 37}},
		Reset: provider.QuotaResetSummary{
			Supported: true, Available: 2,
			Note: "CozyPhi does not perform manual resets; use the official Codex UI.",
		},
	}}
	for i := range 153 {
		msg.Snapshot.Tokens = append(msg.Snapshot.Tokens, provider.QuotaTokenUsage{
			Scope: time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), Tokens: int64(i + 1),
		})
	}
	return msg
}

func draw80(p *Pane, height int) string {
	return components.SurfaceText(p.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: height}}))
}

func TestFullReportScrolling(t *testing.T) {
	p, _, _ := newTestPane()
	p.Show()
	msg := largeQuota()
	p.Apply(msg)
	top := draw80(p, 24)
	assert.Contains(t, top, "Subscription")
	assert.NotContains(t, top, "Session")
	seen := top
	for range 180 {
		require.True(t, press(t, p, xui.KeyDown, 0))
		text := draw80(p, 24)
		assert.Contains(t, text, "Usage — subscription and session")
		assert.Contains(t, text, "Esc close · r refresh")
		seen += text
	}
	for _, usage := range msg.Snapshot.Tokens {
		assert.Contains(t, seen, "tokens ("+usage.Scope+")")
	}
	bottom := draw80(p, 24)
	assert.Contains(t, bottom, "manual resets  2 available")
	assert.Contains(t, bottom, "reset action: CozyPhi does not perform manual resets")
	assert.Contains(t, bottom, "Session")
	assert.Contains(t, bottom, "rounds 7")
	assert.Contains(t, bottom, "context 31k / 128k (24%)")
	assert.Contains(t, bottom, "wall ")
	press(t, p, xui.KeyDown, 0)
	assert.Equal(t, bottom, draw80(p, 24), "bottom clamps")
	press(t, p, xui.KeyHome, 0)
	assert.Equal(t, top, draw80(p, 24))
	press(t, p, xui.KeyUp, 0)
	assert.Equal(t, top, draw80(p, 24), "top clamps")
	press(t, p, xui.KeyDown, 0)
	assert.NotEqual(t, top, draw80(p, 24))
	press(t, p, xui.KeyUp, 0)
	assert.Equal(t, top, draw80(p, 24))
	press(t, p, xui.KeyPageDown, 0)
	page := draw80(p, 24)
	assert.NotEqual(t, top, page)
	press(t, p, xui.KeyPageUp, 0)
	assert.Equal(t, top, draw80(p, 24))
	for range 21 {
		press(t, p, xui.KeyDown, 0)
	}
	assert.Equal(t, page, draw80(p, 24), "page moves by viewport height")
	press(t, p, xui.KeyEnd, 0)
	assert.Equal(t, bottom, draw80(p, 24))

	press(t, p, xui.KeyHome, 0)
	for _, wheel := range []int{0, 5, 1000} {
		require.True(
			t,
			p.HandleEvent(&components.EventContext{}, xui.MouseEvent{Button: xui.MouseWheelDown, Wheel: wheel}),
		)
		assert.NotEqual(t, top, draw80(p, 24))
		require.True(
			t,
			p.HandleEvent(&components.EventContext{}, xui.MouseEvent{Button: xui.MouseWheelUp, Wheel: wheel}),
		)
		assert.Equal(t, top, draw80(p, 24))
	}
}

func TestScrollClampsWhenReportChanges(t *testing.T) {
	for _, change := range []string{"refresh", "show", "shrink", "resize"} {
		t.Run(change, func(t *testing.T) {
			p, _, _ := newTestPane()
			p.Show()
			p.Apply(largeQuota())
			draw80(p, 24)
			press(t, p, xui.KeyEnd, 0)
			assert.Contains(t, draw80(p, 24), "Session")
			height := 24
			switch change {
			case "refresh":
				press(t, p, xui.KeyRune, 'r')
			case "show":
				p.Hide()
				p.Show()
			case "shrink":
				p.Apply(controller.UsageQuotaMsg{ProviderID: "openai", Unsupported: true})
			case "resize":
				height = 240
			}
			text := draw80(p, height)
			assert.Contains(t, text, "Subscription")
			assert.Contains(t, text, "Session")
			assert.Contains(t, text, "rounds 7")
			press(t, p, xui.KeyHome, 0)
			assert.Equal(t, text, draw80(p, height))
		})
	}
}

func TestResetTimingVisibleAt80Columns(t *testing.T) {
	p, _, _ := newTestPane()
	p.Show()
	msg := largeQuota()
	msg.Snapshot.Limits[0].ResetsAt = time.Date(2035, 12, 25, 15, 4, 0, 0, time.UTC)
	p.Apply(msg)
	text := draw80(p, 24)
	assert.Contains(t, text, "37% used · 63% remaining")
	assert.Contains(t, text, "resets Tue 25 Dec 15:04")
	msg.Snapshot.Limits[0].ResetsAt = time.Time{}
	p.Apply(msg)
	assert.Contains(t, draw80(p, 24), "reset time unavailable")
}

func TestDrawDoesNotScheduleWake(t *testing.T) {
	p, _, _ := newTestPane()
	p.Show()
	p.Apply(largeQuota())
	var wake time.Time
	for _, height := range []int{1, 2, 3, 4, 24} {
		s := p.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: height}, Wake: &wake})
		assert.Equal(t, height, s.Size.Height)
		assert.Contains(t, components.SurfaceText(s), "Usage — subscription and session")
		assert.True(t, wake.IsZero(), "static report must not schedule periodic redraws")
	}
	ctx := &components.EventContext{}
	require.True(t, p.HandleEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyDown}))
	assert.True(t, ctx.Consume)
	assert.True(t, ctx.Redraw)
}
