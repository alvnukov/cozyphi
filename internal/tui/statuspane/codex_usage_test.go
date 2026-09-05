package statuspane_test

import (
	"fmt"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
)

func TestUsageTabDoesNotClipCodexProfileHistory(t *testing.T) {
	p := pane()
	p.Show(statuspane.Snapshot{Provider: "openai"})
	snapshot := provider.QuotaSnapshot{
		PlanName: "plus",
		Reset:    provider.QuotaResetSummary{Available: 2, Supported: true},
	}
	for i := range 153 {
		snapshot.Tokens = append(
			snapshot.Tokens,
			provider.QuotaTokenUsage{Scope: fmt.Sprintf("observed bucket %03d", i), Tokens: int64(i)},
		)
	}
	p.ApplyQuota(controller.UsageQuotaMsg{ProviderID: "openai", Snapshot: snapshot}, "openai")
	assert.Contains(t, text(p, 80, 24), "observed bucket 000")
	press(p, xui.KeyEnd, 0)
	bottom := text(p, 80, 24)
	assert.Contains(t, bottom, "observed bucket 152")
	assert.Contains(t, bottom, "manual resets  2 available")
	assert.Contains(t, bottom, "Session")
	assert.Contains(t, bottom, "Cost: unavailable")
	press(p, xui.KeyHome, 0)
	assert.Contains(t, text(p, 80, 24), "observed bucket 000")
}
