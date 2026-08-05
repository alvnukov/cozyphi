package tools

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// BashMaxOutputLines is the maximum number of lines kept in displayed bash output.
	BashMaxOutputLines = 1000
	// BashMaxOutputBytes is the maximum bytes kept in displayed bash output (50KB).
	BashMaxOutputBytes = 50 * 1024
)

// FormatBashOutput keeps the last BashMaxOutputLines / BashMaxOutputBytes of
// output (tail). When truncated, the full output is written to a temp file and
// a panda-style notice with the path is appended — no in-UI "Show more".
func FormatBashOutput(output string) string {
	display, path := truncateBashTail(output, BashMaxOutputLines, BashMaxOutputBytes)
	if path == "" {
		return display
	}
	if !strings.Contains(display, path) {
		display += fmt.Sprintf("\n\n[Full output: %s]", path)
	}
	return display
}

func truncateBashTail(output string, maxLines, maxBytes int) (display, fullPath string) {
	if maxLines <= 0 {
		maxLines = BashMaxOutputLines
	}
	if maxBytes <= 0 {
		maxBytes = BashMaxOutputBytes
	}
	totalBytes := len(output)
	lines := strings.Split(output, "\n")
	// Trailing empty from final newline shouldn't inflate the line count for limits.
	totalLines := len(lines)
	if totalLines > 0 && lines[totalLines-1] == "" {
		totalLines--
		lines = lines[:totalLines]
	}

	if totalLines <= maxLines && totalBytes <= maxBytes {
		return output, ""
	}

	path, err := writeBashTempFile(output)
	if err != nil {
		path = ""
	}

	// Keep a tail that respects both limits.
	start := 0
	if totalLines > maxLines {
		start = totalLines - maxLines
	}
	tail := lines[start:]
	for len(tail) > 1 && joinedBytes(tail) > maxBytes {
		tail = tail[1:]
	}
	if len(tail) == 1 && len(tail[0]) > maxBytes {
		s := tail[0]
		tail = []string{s[len(s)-maxBytes:]}
	}

	display = strings.Join(tail, "\n")
	startLine := totalLines - len(tail) + 1
	endLine := totalLines
	if path != "" {
		display += fmt.Sprintf("\n\n[Showing lines %d-%d of %d. Full output: %s]", startLine, endLine, totalLines, path)
	} else {
		display += fmt.Sprintf("\n\n[Showing lines %d-%d of %d. Full output unavailable]", startLine, endLine, totalLines)
	}
	return display, path
}

func joinedBytes(lines []string) int {
	n := 0
	for i, line := range lines {
		n += len(line)
		if i > 0 {
			n++ // newline
		}
	}
	return n
}

func writeBashTempFile(content string) (string, error) {
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return "", err
	}
	path := filepath.Join(os.TempDir(), fmt.Sprintf("phi-bash-%s.log", hex.EncodeToString(id)))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
