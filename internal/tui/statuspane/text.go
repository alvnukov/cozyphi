package statuspane

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/pulseaiclub/xui"
)

// wrapLines preserves long paths and counters on narrow screens. Terminal control
// characters are replaced rather than interpreted, even in injected metadata.
func wrapLines(lines []string, width int, method xui.WidthMethod) []string {
	if width <= 0 {
		return nil
	}
	var out []string
	for _, line := range lines {
		var chunk strings.Builder
		cells := 0
		for _, r := range line {
			if unicode.IsControl(r) {
				r = ' '
			}
			n := xui.StringWidth(string(r), method)
			if cells+n > width && chunk.Len() > 0 {
				out = append(out, chunk.String())
				chunk.Reset()
				cells = 0
			}
			if n > width {
				r, n = '?', 1
			}
			chunk.WriteRune(r)
			cells += n
		}
		out = append(out, chunk.String())
	}
	return out
}

// heatmap groups observed days into calendar weeks. Missing dates inside a week
// are empty, not fabricated activity. Entire empty weeks are omitted for bounded
// all-time rendering; the week labels retain the gaps.
func heatmap(days []Day) []string {
	if len(days) == 0 {
		return []string{"No recorded activity"}
	}
	weeks := make(map[string][7]int)
	peak := 0
	for _, day := range days {
		if day.Date.IsZero() {
			continue
		}
		date := day.Date.UTC()
		weekday := int(date.Weekday())
		start := date.AddDate(0, 0, -weekday).Format(time.DateOnly)
		week := weeks[start]
		week[weekday] += day.Totals.Rounds
		peak = max(peak, week[weekday])
		weeks[start] = week
	}
	keys := make([]string, 0, len(weeks))
	for key := range weeks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := []string{"Week of     S M T W T F S"}
	for _, key := range keys {
		var cells []string
		for _, rounds := range weeks[key] {
			cell := "."
			switch {
			case rounds > 0 && rounds*3 > peak*2:
				cell = "▓"
			case rounds > 0 && rounds*3 > peak:
				cell = "▒"
			case rounds > 0:
				cell = "░"
			}
			cells = append(cells, cell)
		}
		rows = append(rows, fmt.Sprintf("%s  %s", key, strings.Join(cells, " ")))
	}
	return rows
}
