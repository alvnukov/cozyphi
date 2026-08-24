package transcript

import (
	"github.com/pulseaiclub/phi/internal/components"
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
	tail = components.FormatDuration(m.Duration)
	if m.Truncated {
		if tail != "" {
			tail += " · "
		}
		tail += "hit max tokens"
	}
	return label, tail
}
