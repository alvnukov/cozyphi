package statuspane

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
)

// Positioned spans keep calendar geometry and metric columns out of prose wrapping.
type statsSpan struct {
	x     int
	text  string
	style xui.Style
}
type statsRow []statsSpan

func safeText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return ' '
		}
		return r
	}, s)
}

func (p *Pane) drawStats(surface *components.Surface, ctx components.DrawContext, now time.Time) {
	rows := p.statsRows(surface.Size.Width, ctx.Method, now)
	p.contentHeight = len(rows)
	p.clampScroll()
	for y := 0; y < p.height && y+p.scroll < len(rows); y++ {
		for _, span := range rows[y+p.scroll] {
			if span.x >= surface.Size.Width {
				continue
			}
			surface.Print(
				span.x,
				y+2,
				layout.TruncateToWidth(safeText(span.text), surface.Size.Width-span.x, ctx.Method),
				span.style,
				ctx.Method,
			)
		}
	}
}

func (p *Pane) statsRows(width int, method xui.WidthMethod, now time.Time) []statsRow {
	var rows []statsRow
	prose := func(text string, style xui.Style) {
		for _, line := range wrapLines([]string{safeText(text)}, width, method) {
			rows = append(rows, statsRow{{text: line, style: style}})
		}
	}
	nav := "[Overview]  Models"
	if p.models {
		nav = "Overview  [Models]"
	}
	prose(nav, p.theme.Warning)
	periods := []string{"All time", "Last 7 days", "Last 30 days"}
	periods[p.period] = "[" + periods[p.period] + "]"
	prose(strings.Join(periods, "  "), p.theme.Warning)
	if p.history.Unavailable != "" {
		prose(p.history.Unavailable, p.theme.Muted)
		return rows
	}
	h := p.history
	if h.Partial {
		prose("Partial history — some observations are unavailable", p.theme.Warning)
		prose("? no observed rounds; coverage incomplete", p.theme.Warning)
	}
	if !p.models {
		if !p.historySince.IsZero() {
			prose("· outside selected period (cutoff "+p.historySince.Format(time.DateOnly)+")", p.theme.Muted)
		}
		prose("Recorded snapshot · r reloads; no live coverage", p.theme.Muted)
		rows = append(rows, p.calendarRows(width, method, now)...)
	}
	if p.models {
		if len(h.Models) == 0 {
			prose("No recorded models", p.theme.Muted)
		}
		for _, model := range h.Models {
			prose(model.Name, p.theme.Warning)
			prose(formatTotals(model.Totals), p.theme.Foreground)
		}
	} else {
		metrics := activityMetrics(h, now)
		for i := 0; i < len(metrics); i += 2 {
			if width >= 76 {
				col := width / 2
				left := wrapLines([]string{safeText(metrics[i])}, col-2, method)
				right := wrapLines([]string{safeText(metrics[i+1])}, width-col, method)
				for j := range max(len(left), len(right)) {
					row := statsRow{}
					if j < len(left) {
						row = append(row, statsSpan{text: left[j], style: p.theme.Foreground})
					}
					if j < len(right) {
						row = append(row, statsSpan{x: col, text: right[j], style: p.theme.Foreground})
					}
					rows = append(rows, row)
				}
			} else {
				prose(metrics[i], p.theme.Foreground)
				prose(metrics[i+1], p.theme.Foreground)
			}
			rows = append(rows, nil)
		}
		prose("Token breakdown · recorded", p.theme.Warning)
		prose(formatTotals(h.Totals), p.theme.Foreground)
		prose("Cache reads are included in input tokens", p.theme.Muted)
	}
	prose("Selected period only · favorite/most active by rounds", p.theme.Muted)
	prose("Current streak: UTC recorded days ending today or yesterday; period-bounded", p.theme.Muted)
	if h.Totals.Rounds == 0 {
		prose("No recorded activity", p.theme.Muted)
	}
	prose(
		fmt.Sprintf("Unknown usage %d · models %d · dates %d", h.UnknownUsage, h.UnknownModels, h.UnknownDates),
		p.theme.Muted,
	)
	if h.UnknownDates > 0 {
		prose("Unknown-date rounds remain in totals; cannot period-filter", p.theme.Muted)
	}
	for _, warning := range h.Warnings {
		prose(warning, p.theme.Muted)
	}
	prose("Cost: unavailable · cache writes: unavailable", p.theme.Muted)
	return rows
}

// Calendar ends in the current UTC week. Missing weeks are real empty columns,
// never compressed away. A narrow terminal shows fewer recent weeks, not a wrap.
func (p *Pane) calendarRows(width int, method xui.WidthMethod, now time.Time) []statsRow {
	weeks := min(53, max(1, (width-4)/2))
	end := utcDay(now).AddDate(0, 0, -int(now.UTC().Weekday()))
	start := end.AddDate(0, 0, -7*(weeks-1))
	var rows []statsRow
	for _, line := range wrapLines([]string{fmt.Sprintf("Activity · %d recent weeks · %s–%s UTC", weeks, start.Format(time.DateOnly), utcDay(now).Format(time.DateOnly))}, width, method) {
		rows = append(rows, statsRow{{text: line, style: p.theme.Muted}})
	}
	counts := make(map[time.Time]int)
	peak := 0
	for _, day := range p.history.Days {
		date := utcDay(day.Date)
		if day.Date.IsZero() || date.Before(start) || date.Before(p.historySince) || date.After(utcDay(now)) {
			continue
		}
		counts[date] += max(0, day.Totals.Rounds)
		peak = max(peak, counts[date])
	}
	months := statsRow{}
	lastLabel := -4
	for week := range weeks {
		// Label the column containing the first of the month, not the next Sunday.
		date := start.AddDate(0, 0, week*7+6)
		first := week == 0 && date.Month() == date.AddDate(0, 0, 7).Month()
		if first || week > 0 && date.Month() != date.AddDate(0, 0, -7).Month() {
			x := 4 + week*2
			if x-lastLabel >= 4 && x+3 <= width {
				months = append(months, statsSpan{x: x, text: date.Format("Jan"), style: p.theme.Muted})
				lastLabel = x
			}
		}
	}
	rows = append(rows, months)
	for weekday := range 7 {
		row := statsRow{}
		label := [7]string{"", "Mon", "", "Wed", "", "Fri", ""}[weekday]
		row = append(row, statsSpan{text: label, style: p.theme.Muted})
		for week := range weeks {
			date := start.AddDate(0, 0, week*7+weekday)
			n := counts[date]
			level := 0
			if n > 0 {
				level = 1 + (n*3-1)/max(1, peak)
			}
			char := "■"
			switch {
			case date.After(utcDay(now)):
				char = " "
			case date.Before(p.historySince):
				char, level = "·", 0
			case n == 0 && p.history.Partial:
				char = "?"
			}
			row = append(row, statsSpan{x: 4 + week*2, text: char, style: p.activityStyle(level)})
		}
		rows = append(rows, row)
	}
	legend := statsRow{{text: "Less", style: p.theme.Muted}}
	for level := range 4 {
		legend = append(legend, statsSpan{x: 5 + level*2, text: "■", style: p.activityStyle(level)})
	}
	legend = append(legend, statsSpan{x: 13, text: "More · rounds", style: p.theme.Muted})
	rows = append(rows, legend, nil)
	return rows
}

func (p *Pane) activityStyle(level int) xui.Style {
	switch level {
	case 1:
		return xui.Style{Fg: xui.RGBColor(0x87, 0x46, 0x20)}
	case 2:
		return xui.Style{Fg: xui.RGBColor(0xc9, 0x70, 0x30)}
	case 3:
		return xui.Style{Fg: xui.RGBColor(0xff, 0xad, 0x55)}
	default:
		return p.theme.Border
	}
}
