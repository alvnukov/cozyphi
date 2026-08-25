package lsp

import (
	"os"
	"strings"
	"unicode/utf16"
)

// pid returns the harness process id sent inside initialize. It is sent to
// gopls only; it never appears in results, logs, or errors.
func pid() int { return os.Getpid() }

// uriFromPath renders an absolute, cleaned path as a file:// URI.
func uriFromPath(path string) string {
	return "file://" + fileURIEscape(path)
}

// fileURIEscape percent-encodes the path bytes that are unsafe in a URI.
// Paths are absolute on this seam, so the leading slash stays literal.
func fileURIEscape(path string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(path); i++ {
		c := path[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			b.WriteByte(c)
		case c == '/', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0xF])
		}
	}
	return b.String()
}

// utf16Column converts a 1-based code-point column into a 0-based UTF-16 code
// unit column for the wire. gopls negotiates UTF-16 by default; the model
// contract is always code points.
func utf16Column(line string, codePointColumn int) int {
	if codePointColumn <= 1 {
		return 0
	}
	runes := []rune(line)
	if codePointColumn > len(runes)+1 {
		codePointColumn = len(runes) + 1
	}
	units := 0
	for _, r := range runes[:codePointColumn-1] {
		units += len(utf16.Encode([]rune{r}))
	}
	return units
}

// codePointColumn converts a 0-based UTF-16 code unit column into a 1-based
// code-point column for the model contract.
func codePointColumn(line string, utf16Column int) int {
	if utf16Column <= 0 {
		return 1
	}
	runes := []rune(line)
	units := 0
	for i, r := range runes {
		if units >= utf16Column {
			return i + 1
		}
		units += len(utf16.Encode([]rune{r}))
	}
	return len(runes) + 1
}
