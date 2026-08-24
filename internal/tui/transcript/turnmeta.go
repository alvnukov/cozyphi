package transcript

import (
	"fmt"
	"time"

	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tui/tokens"
)

// formatTurnMeta splits the end-of-turn footer, opencode-style: label is the
// model (plus a context bracket when the provider reported usage) and renders
// bright; tail is the wall-clock duration and renders muted. An empty model
// hides the row entirely.
func formatTurnMeta(m session.TurnMeta) (label, tail string) {
	if m.Model == "" {
		return "", ""
	}
	label = m.Model
	if m.Usage.Reported() {
		label += "[" + tokens.FormatTokens(m.Usage.ContextTokens()) + "]"
	}
	tail = formatTurnDuration(m.Duration)
	if m.Truncated {
		if tail != "" {
			tail += " · "
		}
		tail += "hit max tokens"
	}
	return label, tail
}

// formatTurnDuration renders wall time the way opencode does: 4s, 1m 4s,
// 1h 2m. Zero or negative durations render as empty (nothing to say).
func formatTurnDuration(d time.Duration) string {
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
