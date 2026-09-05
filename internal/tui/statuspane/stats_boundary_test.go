package statuspane

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
)

func fixedStatsPane() (*Pane, *time.Time) {
	now := time.Date(2026, time.January, 4, 23, 59, 59, 0, time.UTC)
	p := New(components.DefaultTheme(), nil, nil, nil)
	p.now = func() time.Time { return now }
	p.ConfigureTabs(func() string { return Stats }, nil)
	p.Show(Snapshot{})
	p.ApplyHistory(History{})
	return p, &now
}

func TestStatsFixedMonthColumns(t *testing.T) {
	p, now := fixedStatsPane()
	s := p.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 60}, Method: xui.WidthUnicode})
	rows := strings.Split(components.SurfaceText(s), "\n")
	// Sep 7 2025–Jan 4 2026: independently tabulated Sunday columns
	// containing Oct 1 (Sep 28), Nov 1 (Oct 26), Dec 1 (Nov 30), Jan 1 (Dec 28).
	want := "    Sep   Oct     Nov       Dec     Jan"
	assert.Contains(t, rows, want+" ", "fixed calendar at %s", now)
}

func TestStatsMidnightWakeAndStreak(t *testing.T) {
	p, now := fixedStatsPane()
	p.ApplyHistory(History{Days: []Day{{Date: now.AddDate(0, 0, -1), Totals: Totals{Rounds: 1}}}})
	var wake time.Time
	ctx := components.DrawContext{Max: components.Size{Width: 80, Height: 60}, Method: xui.WidthUnicode, Wake: &wake}
	assert.Contains(t, components.SurfaceText(p.Draw(ctx)), "Current streak (UTC): 1 days")
	midnight := time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, midnight, wake)
	*now = midnight
	wake = time.Time{}
	assert.Contains(t, components.SurfaceText(p.Draw(ctx)), "Current streak (UTC): 0 days")
	assert.Equal(t, midnight.Add(24*time.Hour), wake)
	for _, state := range []string{"models", "config", "hidden"} {
		p.models = state == "models"
		p.tab = Stats
		if state == "config" {
			p.tab = Config
		}
		p.visible = state != "hidden"
		wake = time.Time{}
		p.Draw(ctx)
		assert.True(t, wake.IsZero(), state)
	}
}

func TestStatsPeriodCutoffAndCoverage(t *testing.T) {
	p, now := fixedStatsPane()
	var requests []int
	p.ConfigureHistory(func(days int) { requests = append(requests, days) })
	for _, days := range []int{7, 30} {
		p.handleRune('p')
		assert.Equal(t, days, requests[len(requests)-1])
		assert.Contains(
			t,
			components.SurfaceText(p.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 24}})),
			"Loading history",
		)
		p.ApplyHistory(History{Partial: true})
		rows := p.calendarRows(40, xui.WidthUnicode, *now)
		// First Sunday is Sep 7, outside both periods. Last Sunday is Jan 4, inside.
		assert.Equal(t, "·", rows[3][1].text)
		assert.Equal(t, "?", rows[3][18].text)
		cutoff := p.historySince
		*now = now.Add(24 * time.Hour)
		p.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 24}})
		assert.Equal(t, cutoff, p.historySince, "Draw must not refilter a loaded snapshot")
	}
}

func TestStatsShortViewportScrollKeepsFooter(t *testing.T) {
	for _, width := range []int{80, 40} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			p, _ := fixedStatsPane()
			p.ApplyHistory(History{Partial: true, Warnings: []string{"FINAL COVERAGE WARNING"}})
			ctx := components.DrawContext{Max: components.Size{Width: width, Height: 24}, Method: xui.WidthUnicode}
			full := p.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: width, Height: 100}))
			all := strings.Split(components.SurfaceText(full), "\n")
			first := strings.Split(components.SurfaceText(p.Draw(ctx)), "\n")
			require.Greater(t, p.contentHeight, p.height)
			footer := first[23]
			assert.Contains(t, footer, "p period")
			for _, key := range []xui.KeyCode{xui.KeyPageDown, xui.KeyEnd, xui.KeyPageUp, xui.KeyHome} {
				p.HandleEvent(&components.EventContext{}, xui.KeyEvent{Press: true, Code: key})
				rows := strings.Split(components.SurfaceText(p.Draw(ctx)), "\n")
				assert.Equal(t, first[:2], rows[:2])
				assert.Equal(t, footer, rows[23])
				assert.Equal(t, all[2+p.scroll:2+p.scroll+p.height], rows[2:23], "content never overlaps footer")
				if key == xui.KeyEnd {
					assert.Contains(t, strings.Join(rows, "\n"), "FINAL COVERAGE WARNING")
					assert.Contains(t, strings.Join(rows, "\n"), "Cost: unavailable")
				}
			}
		})
	}
}
