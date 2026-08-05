package chat

import "strings"

// ActiveSlash reports whether the cursor sits in a leading `/command` token.
// Only the first token of the composer value participates (slash commands are
// whole-line). query is the text after '/' up to the cursor.
// start/end are byte offsets to replace on accept (from '/' through cursor).
func ActiveSlash(value string, cursor int) (query string, start, end int, ok bool) {
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(value) {
		cursor = len(value)
	}
	if !strings.HasPrefix(value, "/") {
		return "", 0, 0, false
	}
	if cursor < 1 {
		return "", 0, 0, false
	}
	// First whitespace ends the command token; picker only while editing it.
	for i := 1; i < len(value); i++ {
		c := value[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if cursor > i {
				return "", 0, 0, false
			}
			break
		}
	}
	return value[1:cursor], 0, cursor, true
}
