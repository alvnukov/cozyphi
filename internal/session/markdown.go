package session

import "strings"

// Markdown renders conversation messages as markdown for /export: one
// "## User" / "## Assistant" section per row, and a "## Side question (btw)"
// section for each aside, so a reader of the file can tell the answers the
// conversation went on from and the ones it never saw. Markers (compaction,
// local bash) and rows without text are skipped, so the file reads as a chat
// log.
func Markdown(messages []Message) string {
	var b strings.Builder
	for _, m := range messages {
		var label, text string
		switch m.Role {
		case RoleUser:
			label, text = "User", m.FlatText()
		case RoleAssistant:
			label, text = "Assistant", m.FlatText()
		case RoleAside:
			label, text = "Side question (btw)", asideMarkdown(m.Aside, m.State)
		default:
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		b.WriteString("## " + label + "\n\n" + text + "\n\n")
	}
	// Rows are separated by a blank line; the file ends with one newline.
	return strings.TrimSuffix(b.String(), "\n")
}

// asideMarkdown quotes the question, says when the answer did not finish,
// names what it was about, and follows with the answer.
func asideMarkdown(row AsideRow, state State) string {
	question := strings.TrimSpace(row.Question)
	if question == "" {
		return ""
	}
	parts := []string{"> " + strings.ReplaceAll(question, "\n", "\n> ")}
	switch state {
	case StateCancelled:
		parts = append(parts, "(cancelled)")
	case StateError:
		parts = append(parts, "(failed)")
	}
	if row.AnchorPreview != "" {
		parts = append(parts, "re: "+row.AnchorPreview)
	}
	if answer := strings.TrimSpace(row.Answer); answer != "" {
		parts = append(parts, answer)
	}
	if row.SkippedTool != "" {
		parts = append(parts, AsideSkippedToolNote(row.SkippedTool))
	}
	if msg := strings.TrimSpace(row.Error); msg != "" {
		parts = append(parts, msg)
	}
	return strings.Join(parts, "\n\n")
}
