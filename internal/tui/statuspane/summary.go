package statuspane

import (
	"fmt"
	"strings"
	"time"
)

// utcDay normalizes recorded timestamps before calendar and streak arithmetic.
func utcDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// activityMetrics derives period-bounded observations, never session durations
// or activity for unknown dates. The current streak is relative to the UTC day.
func activityMetrics(h History, now time.Time) []string {
	counts := make(map[time.Time]int)
	for _, day := range h.Days {
		if !day.Date.IsZero() && day.Totals.Rounds > 0 {
			counts[utcDay(day.Date)] += day.Totals.Rounds
		}
	}
	longest, current := 0, 0
	most, peak := time.Time{}, 0
	for date, count := range counts {
		if count > peak || count == peak && (most.IsZero() || date.Before(most)) {
			most, peak = date, count
		}
		if counts[date.AddDate(0, 0, -1)] > 0 {
			continue
		}
		streak := 0
		for d := date; counts[d] > 0; d = d.AddDate(0, 0, 1) {
			streak++
		}
		longest = max(longest, streak)
	}
	end := utcDay(now)
	if counts[end] == 0 {
		end = end.AddDate(0, 0, -1)
	}
	for d := end; counts[d] > 0; d = d.AddDate(0, 0, -1) {
		current++
	}
	favorite, rounds := "unavailable", 0
	for _, model := range h.Models {
		if strings.TrimSpace(model.Name) == "" || model.Name == "unknown" {
			continue
		}
		if model.Totals.Rounds > rounds || model.Totals.Rounds == rounds && rounds > 0 && model.Name < favorite {
			favorite, rounds = model.Name, model.Totals.Rounds
		}
	}
	date := "unavailable"
	if !most.IsZero() {
		date = most.Format(time.DateOnly)
	}
	return []string{
		"Favorite model: " + favorite, fmt.Sprintf("Total tokens: %d", h.Totals.Total),
		fmt.Sprintf("Sessions: %d", h.Totals.Sessions), "Longest session: unavailable",
		fmt.Sprintf("Active days (UTC): %d", len(counts)), fmt.Sprintf("Longest recorded streak: %d days", longest),
		"Most active date: " + date, fmt.Sprintf("Current streak (UTC): %d days", current),
	}
}
