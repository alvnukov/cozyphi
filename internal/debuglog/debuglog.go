package debuglog

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var (
	mu      sync.Mutex
	file    *os.File
	enabled bool
	checked bool
)

// Enabled reports whether debug logging is on (cached after first check).
func Enabled() bool {
	mu.Lock()
	defer mu.Unlock()
	if !checked {
		checked = true
		enabled = os.Getenv("PHI_DEBUG") == "1" || strings.EqualFold(os.Getenv("PHI_DEBUG"), "true")
	}
	return enabled
}

func openLocked() error {
	if file != nil {
		return nil
	}
	path := os.Getenv("PHI_DEBUG_FILE")
	if path == "" {
		path = "phi-debug.log"
	}
	//nolint:gosec // G703: path comes from PHI_DEBUG_FILE or a fixed default
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		enabled = false
		return err
	}
	file = f
	_, _ = fmt.Fprintf(file, "\n---- debug session %s ----\n", time.Now().Format(time.RFC3339))
	return nil
}

// Logf writes a timestamped line when PHI_DEBUG is set.
func Logf(format string, args ...any) {
	if !Enabled() {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if err := openLocked(); err != nil {
		return
	}
	_, _ = fmt.Fprintf(file, "%s ", time.Now().Format("15:04:05.000"))
	_, _ = fmt.Fprintf(file, format, args...)
	if format == "" || format[len(format)-1] != '\n' {
		_, _ = fmt.Fprintln(file)
	}
}

// DumpRunes logs each rune with hex and display-oriented notes.
func DumpRunes(label, s string) {
	if !Enabled() {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s len=%d runes=%d\n", label, len(s), utf8.RuneCountInString(s))
	i := 0
	for _, r := range s {
		fmt.Fprintf(&b, "  [%d] U+%04X %q", i, r, string(r))
		switch {
		case r == '\n':
			b.WriteString(" <NL>")
		case r == '\t':
			b.WriteString(" <TAB>")
		case r < 0x20 || r == 0x7f:
			b.WriteString(" <CTRL>")
		case r >= 0x2e80 && r <= 0xa4cf, r >= 0x3400 && r <= 0x4dbf, r >= 0x20000 && r <= 0x3fffd:
			b.WriteString(" <WIDE?>")
		}
		b.WriteByte('\n')
		i++
	}
	Logf("%s", b.String())
}
