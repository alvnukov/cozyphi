package statuspane_test

import (
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
)

func TestStatsRecordedCurrentStreakAndModels(t *testing.T) {
	now := time.Now().UTC()
	p := pane()
	p.ConfigureTabs(func() string { return statuspane.Stats }, nil)
	p.Show(statuspane.Snapshot{})
	h := statuspane.History{
		Days: []statuspane.Day{
			{Date: now.AddDate(0, 0, -1), Totals: statuspane.Totals{Rounds: 1}},
			{Date: now.AddDate(0, 0, -2), Totals: statuspane.Totals{Rounds: 2}},
		},
		Models: []statuspane.Model{
			{Name: "alpha", Totals: statuspane.Totals{Rounds: 1, Input: 10, Output: 20, Cached: 5, Total: 30}},
			{Name: "beta", Totals: statuspane.Totals{Rounds: 2, Input: 40, Output: 50, Cached: 6, Total: 90}},
		},
	}
	p.ApplyHistory(h)
	assert.Contains(t, text(p, 100, 40), "Current streak (UTC): 2 days")
	h.Days[0].Date = now.AddDate(0, 0, -4)
	p.ApplyHistory(h)
	assert.Contains(t, text(p, 100, 40), "Current streak (UTC): 0 days")
	press(p, xui.KeyRune, 'm')
	rendered := text(p, 100, 40)
	assert.Contains(t, rendered, "Overview  [Models]")
	assert.Contains(t, rendered, "alpha")
	assert.Contains(t, rendered, "1 rounds · in 10 · out 20 · cache read 5 · total 30")
	assert.Contains(t, rendered, "beta")
	assert.Contains(t, rendered, "2 rounds · in 40 · out 50 · cache read 6 · total 90")
	assert.NotContains(t, rendered, "Activity ·")
}
