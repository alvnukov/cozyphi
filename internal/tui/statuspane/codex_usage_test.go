package statuspane_test

import (
	"fmt"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
)

func TestUsageTabKeepsCodexProfileHistoryOutOfCompactSummary(t *testing.T) {
	p := statuspane.New(components.DefaultTheme(), func() controller.SessionStats {
		return controller.SessionStats{Model: "codex", ProviderID: "openai", Rounds: 9}
	}, nil, nil)
	p.Show(statuspane.Snapshot{Provider: "openai"})
	snapshot := provider.QuotaSnapshot{
		PlanName:    "plus",
		Reset:       provider.QuotaResetSummary{Available: 2, Supported: true},
		ResetTarget: &provider.QuotaResetTarget{},
		Tokens:      []provider.QuotaTokenUsage{{Scope: "Codex profile lifetime", Tokens: 9999}},
	}
	for i := range 153 {
		snapshot.Tokens = append(
			snapshot.Tokens,
			provider.QuotaTokenUsage{Scope: fmt.Sprintf("observed bucket %03d", i), Tokens: int64(i)},
		)
	}
	p.ApplyQuota(controller.UsageQuotaMsg{ProviderID: "openai", Snapshot: snapshot}, "openai")
	top := text(p, 80, 24)
	assert.Contains(t, top, "plan  plus")
	assert.Contains(t, top, "limit resets  2 available")
	assert.Contains(t, top, "Session")
	assert.Contains(t, top, "rounds 9")
	assert.Contains(t, top, "Cost: unavailable")
	assert.NotContains(t, top, "observed bucket")
	assert.NotContains(t, top, "lifetime")
	assert.NotContains(t, top, "token data unavailable")
	assert.NotContains(t, top, "Reset limit")
	press(p, xui.KeyEnd, 0)
	assert.Equal(t, top, text(p, 80, 24), "compact summary fits without scrolling")
	press(p, xui.KeyRune, 'x')
	press(p, xui.KeyRune, 'y')
	assert.NotContains(t, text(p, 80, 24), "Spend one reset credit")
	press(p, xui.KeyHome, 0)
	assert.Equal(t, top, text(p, 80, 24))
}
