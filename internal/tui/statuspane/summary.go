package statuspane

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// recordedSummary uses only the selected snapshot, never today's date or inferred activity.
func recordedSummary(h History) []string {
	active := make(map[time.Time]struct{})
	for _, day := range h.Days {
		if day.Date.IsZero() || day.Totals.Rounds <= 0 {
			continue
		}
		date := day.Date.UTC()
		active[time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)] = struct{}{}
	}
	dates := make([]time.Time, 0, len(active))
	for date := range active {
		dates = append(dates, date)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
	longest, streak := 0, 0
	for i, date := range dates {
		if i > 0 && dates[i-1].AddDate(0, 0, 1).Equal(date) {
			streak++
		} else {
			streak = 1
		}
		longest = max(longest, streak)
	}
	favorite, rounds := "", 0
	for _, model := range h.Models {
		if strings.TrimSpace(model.Name) == "" || model.Name == "unknown" || model.Totals.Rounds <= 0 {
			continue
		}
		if model.Totals.Rounds > rounds || model.Totals.Rounds == rounds && model.Name < favorite {
			favorite, rounds = model.Name, model.Totals.Rounds
		}
	}
	if favorite == "" {
		favorite = "unavailable"
	}
	return []string{
		"Recorded activity · selected period",
		fmt.Sprintf("Active UTC days: %d", len(active)),
		fmt.Sprintf("Longest recorded-day streak: %d days", longest),
		"Favorite known model (rounds): " + favorite,
	}
}
