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

func isSlashSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// ActiveSlashArg reports whether the cursor sits in an argument of a leading
// "/cmd arg…" line (the command token is already complete). name is the
// command without the slash, args the completed arguments before the cursor,
// partial the argument text under the cursor up to it; start/end replace the
// whole argument token on accept.
func ActiveSlashArg(value string, cursor int) (name string, args []string, partial string, start, end int, ok bool) {
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(value) {
		cursor = len(value)
	}
	if !strings.HasPrefix(value, "/") {
		return "", nil, "", 0, 0, false
	}
	cmdEnd := 1
	for cmdEnd < len(value) && !isSlashSpace(value[cmdEnd]) {
		cmdEnd++
	}
	// No whitespace yet: the name token is still being edited.
	if cmdEnd == 1 || cmdEnd == len(value) {
		return "", nil, "", 0, 0, false
	}
	// Walk the argument tokens until one holds the cursor; the ones before
	// it are the context a completer needs to know which argument this is.
	for i := cmdEnd; ; {
		for i < len(value) && isSlashSpace(value[i]) {
			i++
		}
		start = i
		for i < len(value) && !isSlashSpace(value[i]) {
			i++
		}
		end = i
		if cursor > end {
			args = append(args, value[start:end])
			continue
		}
		if cursor < start {
			// The cursor sits in whitespace before the token, so no
			// argument is being edited.
			return "", nil, "", 0, 0, false
		}
		return value[1:cmdEnd], args, value[start:cursor], start, end, true
	}
}
