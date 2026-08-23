package session

import "strings"

// Markdown renders conversation messages as markdown for /export: one
// "## User" / "## Assistant" section per row. Markers (compaction, local
// bash) and rows without text are skipped, so the file reads as a chat log.
func Markdown(messages []Message) string {
	var b strings.Builder
	for _, m := range messages {
		var label string
		switch m.Role {
		case RoleUser:
			label = "User"
		case RoleAssistant:
			label = "Assistant"
		default:
			continue
		}
		text := strings.TrimSpace(m.FlatText())
		if text == "" {
			continue
		}
		b.WriteString("## " + label + "\n\n" + text + "\n\n")
	}
	// Rows are separated by a blank line; the file ends with one newline.
	return strings.TrimSuffix(b.String(), "\n")
}
