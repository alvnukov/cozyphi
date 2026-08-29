package transcript

import (
	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/tokens"
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

// formatItemMeta keeps the live and terminal labels on one row: while the
// assistant is streaming, the model is the stable identity and "thinking" is
// its current state; completion replaces that state with the turn's metrics.
func formatItemMeta(it session.Item) (label, tail string) {
	label, tail = formatTurnMeta(it.TurnMeta)
	if label != "" && it.State == session.StateStreaming {
		tail = "thinking"
	}
	return label, tail
}
