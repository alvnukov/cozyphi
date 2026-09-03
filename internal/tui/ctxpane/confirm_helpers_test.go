package ctxpane

import "strings"

// The pane keeps one Confirm for both questions; which one is armed shows
// only in the label, which is exactly what the user sees.
func trimArmed(p *Pane) bool {
	return p.confirm.Armed() && strings.HasPrefix(p.confirm.Label(), "trim")
}

func deleteArmed(p *Pane) bool {
	return p.confirm.Armed() && strings.HasPrefix(p.confirm.Label(), "delete")
}
