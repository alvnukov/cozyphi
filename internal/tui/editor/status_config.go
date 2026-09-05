package editor

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/job"
)

// Select scalars immediately: Manager.Snapshot's AgentModels map can alias the
// manager. The dashboard retains neither that map nor a persistence capability.
func statusSettingsRows(s harnesssettings.Snapshot) []string {
	reminder := fmt.Sprintf("%d tokens", s.Compaction.ReminderTokens)
	if s.Compaction.ReminderTokens == 0 {
		reminder = "effective unavailable; configured automatic default"
	}
	notifications := s.Notifications.Mode.String()
	rows := []string{
		"Settings snapshot · at dashboard open",
		fmt.Sprintf("OpenCode import enabled: %t", s.OpenCodeEnabled),
		"Compaction reminder: " + reminder,
		"Notifications: " + notifications,
		fmt.Sprintf("Notification sound enabled: %t", s.Notifications.Sound != ""),
		fmt.Sprintf("Configured task access: %s", s.Tasks.Normalized()),
		fmt.Sprintf("Configured plan step types: %d", len(s.Plan.Types)),
		fmt.Sprintf("Additional plan-gate exemptions: %d", len(s.Plan.AdditionalExemptions)),
		"Configured agent model pins · effective unavailable",
	}
	for _, role := range job.Roles() {
		model := s.AgentModels[string(role)]
		if model == "" {
			model = "inherit session model"
		}
		rows = append(rows, "  "+string(role)+": "+statusModelPin(model))
	}
	return append(rows, "Pins may fall back when unavailable; running agents retain their model.")
}

// Model identifiers are display data, not URLs, environment expressions or
// terminal programs. Do not render arbitrary configured strings as identifiers.
func statusModelPin(s string) string {
	if strings.Contains(s, "://") || strings.ContainsAny(s, "$=@\\") {
		return "[invalid model pin]"
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return ' '
		}
		return r
	}, s)
}
