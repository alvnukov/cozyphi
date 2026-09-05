package statuspane_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
)

func TestStatsOverviewRecordedSummary(t *testing.T) {
	day := func(date string, rounds int) statuspane.Day {
		parsed, err := time.Parse(time.RFC3339, date)
		if err != nil {
			t.Fatal(err)
		}
		return statuspane.Day{Date: parsed, Totals: statuspane.Totals{Rounds: rounds}}
	}
	model := func(name string, rounds int) statuspane.Model {
		return statuspane.Model{Name: name, Totals: statuspane.Totals{Rounds: rounds}}
	}
	for _, tt := range []struct {
		name           string
		history        statuspane.History
		active, streak int
		favorite       string
	}{
		{name: "empty", favorite: "unavailable"},
		{name: "unordered duplicates UTC and gaps", history: statuspane.History{Days: []statuspane.Day{
			day("2024-03-03T12:00:00Z", 1), day("2024-02-29T00:00:00Z", 2),
			day("2024-03-01T00:30:00+02:00", 3), day("2024-03-01T23:00:00Z", 1),
			day("2024-03-02T00:00:00Z", 0),
			{Totals: statuspane.Totals{Rounds: 99}},
		}}, active: 3, streak: 2, favorite: "unavailable"},
		{name: "favorite rounds not tokens", history: statuspane.History{Models: []statuspane.Model{
			model("beta", 4), {Name: "alpha", Totals: statuspane.Totals{Rounds: 3, Total: 999}}, model("unknown", 100),
		}}, favorite: "beta"},
		{name: "lexical tie", history: statuspane.History{Models: []statuspane.Model{model("beta", 4), model("alpha", 4)}}, favorite: "alpha"},
		{name: "reverse lexical tie", history: statuspane.History{Models: []statuspane.Model{model("alpha", 4), model("beta", 4)}}, favorite: "alpha"},
		{name: "unknown and inactive models", history: statuspane.History{Models: []statuspane.Model{model("unknown", 10), model("", 20), model("  ", 30), model("idle", 0)}}, favorite: "unavailable"},
		{name: "partial", history: statuspane.History{Partial: true, UnknownDates: 2, UnknownModels: 3}, favorite: "unavailable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := pane()
			p.ConfigureTabs(func() string { return statuspane.Stats }, nil)
			p.Show(statuspane.Snapshot{})
			p.ApplyHistory(tt.history)
			rendered := text(p, 140, 50)
			assert.Contains(t, rendered, "Recorded activity · selected period")
			assert.Contains(t, rendered, fmt.Sprintf("Active UTC days: %d", tt.active))
			assert.Contains(t, rendered, fmt.Sprintf("Longest recorded-day streak: %d days", tt.streak))
			assert.Contains(t, rendered, "Favorite known model (rounds): "+tt.favorite)
			if tt.history.Partial {
				assert.Contains(t, rendered, "Partial history")
				assert.Contains(t, rendered, "models 3 · dates 2")
			}
		})
	}
}
