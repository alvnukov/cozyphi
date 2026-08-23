package transcript

import (
	"fmt"
	"time"

	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tui/tokens"
)

// formatTurnMeta renders the end-of-turn metadata row, opencode-style:
// "• model[context] • 1m 4s". The context bracket appears only when the
// provider reported usage; an empty model hides the row entirely.
func formatTurnMeta(m session.TurnMeta) string {
	if m.Model == "" {
		return ""
	}
	meta := "• " + m.Model
	if m.Usage.Reported() {
		meta += "[" + tokens.FormatTokens(m.Usage.ContextTokens()) + "]"
	}
	if d := formatTurnDuration(m.Duration); d != "" {
		meta += " • " + d
	}
	return meta
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
