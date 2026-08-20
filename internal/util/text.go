package util

import "strings"

// NormalizeLF converts CRLF and lone CR line endings to LF.
func NormalizeLF(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}
