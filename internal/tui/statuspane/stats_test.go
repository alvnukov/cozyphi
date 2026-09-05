package statuspane_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
)

func cell(s components.Surface, x, y int) xui.Cell { return s.Buffer[y*s.Size.Width+x] }

func rowContaining(t *testing.T, s components.Surface, text string) int {
	t.Helper()
	for y, row := range strings.Split(components.SurfaceText(s), "\n") {
		if strings.Contains(row, text) {
			return y
		}
	}
	t.Fatalf("missing row %q", text)
	return 0
}

func draw(p *statuspane.Pane, width int) components.Surface {
	return p.Draw(components.DrawContext{Max: components.Size{Width: width, Height: 60}, Method: xui.WidthUnicode})
}

func TestStatsVisualCalendarColumnsColorsAndResponsiveMetrics(t *testing.T) {
	now := time.Now().UTC()
	sunday := time.Date(now.Year(), now.Month(), now.Day()-int(now.Weekday()), 0, 0, 0, 0, time.UTC)
	p := pane()
	p.ConfigureTabs(func() string { return statuspane.Stats }, nil)
	p.Show(statuspane.Snapshot{})
	p.ApplyHistory(statuspane.History{
		Totals: statuspane.Totals{Sessions: 3, Rounds: 6, Input: 1200, Output: 300, Cached: 400, Total: 1500},
		Days: []statuspane.Day{
			{Date: sunday.AddDate(0, 0, -28), Totals: statuspane.Totals{Rounds: 1}},
			{Date: sunday.AddDate(0, 0, -14), Totals: statuspane.Totals{Rounds: 2}},
			{Date: sunday.AddDate(0, 0, -7), Totals: statuspane.Totals{Rounds: 3}},
		},
		Models:  []statuspane.Model{{Name: "recorded-model", Totals: statuspane.Totals{Rounds: 6, Total: 1500}}},
		Partial: true, UnknownDates: 1,
	})
	for _, width := range []int{112, 40} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			s := draw(p, width)
			weeks := min(53, (width-4)/2)
			mon := rowContaining(t, s, "Mon")
			sun := mon - 1
			assert.Equal(t, "M", cell(s, 0, mon).Char)
			assert.Equal(t, "W", cell(s, 0, mon+2).Char)
			assert.Equal(t, "F", cell(s, 0, mon+4).Char)
			theme := components.DefaultTheme()
			for day := range 7 {
				for week := range weeks - 1 {
					want := "?"
					if day == 0 && (week == weeks-5 || week == weeks-3 || week == weeks-2) {
						want = "■"
					}
					assert.Equal(t, want, cell(s, 4+week*2, sun+day).Char)
					assert.Equal(t, " ", cell(s, 5+week*2, sun+day).Char)
				}
			}
			// The empty intervening week occupies all seven cells, not a skipped column.
			gapX := 4 + 2*(weeks-4)
			for day := range 7 {
				assert.Equal(t, theme.Border.Fg, cell(s, gapX, sun+day).Style.Fg)
			}
			colors := []xui.Style{
				{Fg: xui.RGBColor(0x87, 0x46, 0x20)},
				{Fg: xui.RGBColor(0xc9, 0x70, 0x30)},
				{Fg: xui.RGBColor(0xff, 0xad, 0x55)},
			}
			for i, ago := range []int{4, 2, 1} {
				assert.Equal(t, colors[i].Fg, cell(s, 4+2*(weeks-1-ago), sun).Style.Fg)
			}
			legend := rowContaining(t, s, "Less")
			assert.Equal(t, sun+7, legend)
			assert.Equal(t, "L", cell(s, 0, legend).Char)
			assert.Equal(t, "M", cell(s, 13, legend).Char)
			assert.Equal(t, theme.Border.Fg, cell(s, 5, legend).Style.Fg)
			for i, style := range colors {
				assert.Equal(t, style.Fg, cell(s, 7+2*i, legend).Style.Fg)
			}
			favorite := rowContaining(t, s, "Favorite model:")
			tokens := rowContaining(t, s, "Total tokens:")
			if width >= 76 {
				assert.Equal(t, favorite, tokens)
				assert.Equal(t, "T", cell(s, width/2, tokens).Char)
				assert.Equal(t, theme.Foreground.Fg, cell(s, width/2, tokens).Style.Fg)
				assert.Equal(t, rowContaining(t, s, "Sessions:"), rowContaining(t, s, "Longest session:"))
			} else {
				assert.Equal(t, favorite+1, tokens)
				assert.Equal(t, "T", cell(s, 0, tokens).Char)
			}
			assert.Contains(t, components.SurfaceText(s), fmt.Sprintf("%d recent weeks", weeks))
			assert.Contains(t, components.SurfaceText(s), "[All time]")
			assert.Contains(t, components.SurfaceText(s), "Last 7 days")
			assert.Contains(t, components.SurfaceText(s), "Last 30 days")
			partial := rowContaining(t, s, "Partial history")
			assert.Equal(t, theme.Warning.Fg, cell(s, 0, partial).Style.Fg)
			assert.Less(t, partial, sun)
			assert.Less(t, rowContaining(t, s, "? no observed rounds"), sun)
			t.Logf(
				"Stats %d columns (test fixture; orange levels RGB 874620/c97030/ffad55):\n%s",
				width,
				components.SurfaceText(s),
			)
		})
	}
}

func TestStatsEmptyAndUnavailableAreNotInventedActivity(t *testing.T) {
	p := pane()
	p.ConfigureTabs(func() string { return statuspane.Stats }, nil)
	p.Show(statuspane.Snapshot{})
	p.ApplyHistory(statuspane.History{})
	s := draw(p, 80)
	sun := rowContaining(t, s, "Mon") - 1
	for day := range 7 {
		for week := range 37 {
			assert.Equal(t, "■", cell(s, 4+2*week, sun+day).Char)
			assert.Equal(t, components.DefaultTheme().Border.Fg, cell(s, 4+2*week, sun+day).Style.Fg)
		}
	}
	assert.Contains(t, components.SurfaceText(s), "No recorded activity")
	assert.Contains(t, components.SurfaceText(s), "Longest session: unavailable")
	assert.Contains(t, components.SurfaceText(s), "Current streak (UTC): 0 days")
	p.ApplyHistory(statuspane.History{Unavailable: "Cannot load history"})
	s = draw(p, 80)
	require.Contains(t, components.SurfaceText(s), "Cannot load history")
	assert.NotContains(t, components.SurfaceText(s), "Sessions: 0")
	assert.NotContains(t, components.SurfaceText(s), "■")
	assert.NotContains(t, components.SurfaceText(s), "Activity ·")
	p.ConfigureHistory(func(int) {})
	p.Show(statuspane.Snapshot{})
	text := components.SurfaceText(draw(p, 80))
	assert.Contains(t, text, "Loading history")
	assert.NotContains(t, text, "■")
	for _, width := range []int{1, 2, 4, 8, 16} {
		s = draw(p, width)
		assert.Equal(t, width, s.Size.Width)
	}
}
