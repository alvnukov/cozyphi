package components

import (
	"fmt"
	"time"
)

// FormatDuration renders wall time the way opencode does: 4s, 1m 4s, 1h 2m.
// Zero or negative durations render as empty (nothing to say). Shared by the
// turn footer and the thinking header so both tell time the same way.
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
}
