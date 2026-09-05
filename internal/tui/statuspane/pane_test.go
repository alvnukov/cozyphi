package statuspane_test

import (
	"strings"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
)

func press(p *statuspane.Pane, code xui.KeyCode, r rune) {
	p.HandleEvent(&components.EventContext{}, xui.KeyEvent{Press: true, Code: code, Rune: r})
}

func text(p *statuspane.Pane, w, h int) string {
	return components.SurfaceText(p.Draw(components.DrawContext{
		Max: components.Size{Width: w, Height: h}, Method: xui.WidthUnicode,
	}))
}

func pane() *statuspane.Pane {
	return statuspane.New(components.DefaultTheme(), func() controller.SessionStats {
		return controller.SessionStats{Model: "model-a", ProviderID: "provider-a", Rounds: 9, InputTokens: 3456}
	}, nil, nil)
}

func TestTabsPreferencesAndInputOwnership(t *testing.T) {
	p := pane()
	var closed []string
	choices := 0
	p.ConfigureTabs(func() string { choices++; return "not-a-tab" }, func(tab string) { closed = append(closed, tab) })
	p.Show(statuspane.Snapshot{})
	require.Equal(t, statuspane.Usage, p.Tab())
	assert.Equal(t, 1, choices)
	press(p, xui.KeyF3, 0)
	assert.Equal(t, statuspane.Stats, p.Tab())
	press(p, xui.KeyF3, 0)
	assert.Equal(t, statuspane.Status, p.Tab())
	press(p, xui.KeyF2, 0)
	assert.Equal(t, statuspane.Stats, p.Tab())
	ctx := &components.EventContext{}
	assert.True(t, p.HandleEvent(ctx, xui.MouseEvent{}))
	assert.True(t, ctx.Consume)
	press(p, xui.KeyEscape, 0)
	p.Hide()
	assert.Equal(t, []string{statuspane.Stats}, closed)
	assert.False(t, p.Visible())
}

func TestUsageRejectsStaleProvidersAndDrawDoesNotFetch(t *testing.T) {
	refreshes := 0
	p := statuspane.New(components.DefaultTheme(), nil, func() { refreshes++ }, nil)
	p.Show(statuspane.Snapshot{Model: "a", Provider: "a"})
	p.ApplyQuota(
		controller.UsageQuotaMsg{ProviderID: "old", Snapshot: provider.QuotaSnapshot{PlanName: "OLD-PLAN"}},
		"a",
	)
	assert.NotContains(t, text(p, 100, 30), "OLD-PLAN")
	p.ApplyQuota(
		controller.UsageQuotaMsg{ProviderID: "a", Snapshot: provider.QuotaSnapshot{PlanName: "CURRENT-PLAN"}},
		"a",
	)
	assert.Contains(t, text(p, 100, 30), "CURRENT-PLAN")
	p.SetModel("b", "b")
	assert.NotContains(t, text(p, 100, 30), "CURRENT-PLAN")
	assert.Contains(t, text(p, 100, 30), "r refresh")
	assert.Equal(t, 2, refreshes, "only opening and the stale response retry start work, not drawing")
	press(p, xui.KeyRune, 'r')
	assert.Equal(t, 3, refreshes)
	p.ApplyQuota(controller.UsageQuotaMsg{ProviderID: "a", Snapshot: provider.QuotaSnapshot{PlanName: "OLD-PLAN"}}, "b")
	assert.NotContains(t, text(p, 100, 30), "OLD-PLAN")
	assert.Contains(t, text(p, 100, 30), "Cost: unavailable")
	p.Hide()
	p.ApplyQuota(controller.UsageQuotaMsg{ProviderID: "b"}, "b")
}

func TestStatsPeriodsOverviewModelsAndDetachedHistory(t *testing.T) {
	p := pane()
	var periods []int
	p.ConfigureTabs(func() string { return statuspane.Stats }, nil)
	p.ConfigureHistory(func(days int) { periods = append(periods, days) })
	p.Show(statuspane.Snapshot{})
	assert.Contains(t, text(p, 100, 30), "Loading history")
	h := statuspane.History{
		Totals: statuspane.Totals{Sessions: 3, Rounds: 12, Input: 500},
		Days: []statuspane.Day{
			{Date: time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC), Totals: statuspane.Totals{Rounds: 12}},
		},
		Models:       []statuspane.Model{{Name: "recorded-model", Totals: statuspane.Totals{Rounds: 12}}},
		Partial:      true,
		UnknownDates: 2,
	}
	p.ApplyHistory(h)
	h.Models[0].Name = "mutated"
	rendered := text(p, 120, 40)
	assert.Contains(t, rendered, "Sessions: 3")
	assert.Contains(t, rendered, "2026-05-10")
	assert.Contains(t, rendered, "■")
	assert.Contains(t, rendered, "Partial history")
	press(p, xui.KeyRune, 'm')
	assert.Contains(t, text(p, 120, 40), "recorded-model")
	for range 3 {
		press(p, xui.KeyRune, 'p')
	}
	assert.Equal(t, []int{0, 7, 30, 0}, periods)
}

func TestResizeScrollAndStatusSnapshot(t *testing.T) {
	p := pane()
	p.ConfigureTabs(func() string { return statuspane.Status }, nil)
	snapshot := statuspane.Snapshot{
		Session: "session-real",
		CWD:     "/long/directory/path",
		MCP:     []string{"server: ready"},
	}
	p.Show(snapshot)
	snapshot.MCP[0] = "mutated"
	assert.Contains(t, text(p, 100, 30), "server: ready")
	assert.Contains(t, text(p, 100, 30), "Account: unavailable")
	for _, size := range []components.Size{{}, {Width: 1, Height: 1}, {Width: 2, Height: 2}, {Width: 8, Height: 4}} {
		s := p.Draw(components.DrawContext{Max: size, Method: xui.WidthUnicode})
		assert.Equal(t, size, s.Size)
	}
	text(p, 12, 5)
	press(p, xui.KeyEnd, 0)
	assert.NotContains(t, text(p, 12, 5), "session-real")
	press(p, xui.KeyHome, 0)
	assert.Contains(t, strings.ReplaceAll(text(p, 12, 10), "\n", ""), "session-real")
}
