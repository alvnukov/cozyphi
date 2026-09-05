package project

// PreferredStatusTab chooses the most frequently closed dashboard tab. Usage
// wins a tie; the remaining ties follow the dashboard's visual order.
func (s UIState) PreferredStatusTab() string {
	best := "usage"
	for _, tab := range []string{"status", "config", "stats"} {
		if s.StatusCloses[tab] > s.StatusCloses[best] {
			best = tab
		}
	}
	return best
}

// RecordStatusClose records one actual dashboard closure. Unknown names are
// ignored so a stale or malformed tab cannot become the opening preference.
func (s *UIState) RecordStatusClose(tab string) {
	switch tab {
	case "status", "config", "usage", "stats":
	default:
		return
	}
	if s.StatusCloses == nil {
		s.StatusCloses = make(map[string]uint64)
	}
	// Saturating avoids wrapping a persisted counter back to zero.
	if s.StatusCloses[tab] < ^uint64(0) {
		s.StatusCloses[tab]++
	}
}
