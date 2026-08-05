package tools

import (
	"os"
	"strings"
	"testing"
)

func TestFormatBashOutputShort(t *testing.T) {
	in := "a\nb\nc\n"
	got := FormatBashOutput(in)
	if got != in {
		t.Fatalf("short output changed: %q", got)
	}
}

func TestFormatBashOutputWritesTemp(t *testing.T) {
	var b strings.Builder
	for i := 0; i < BashMaxOutputLines+20; i++ {
		b.WriteString("line\n")
	}
	full := b.String()
	got := FormatBashOutput(full)
	if !strings.Contains(got, "Full output:") {
		t.Fatalf("missing full-output notice: %q", got)
	}
	if !strings.Contains(got, "Showing lines") {
		t.Fatalf("missing range notice: %q", got)
	}
	// Extract path and confirm file exists with full content.
	idx := strings.Index(got, "Full output: ")
	rest := got[idx+len("Full output: "):]
	path := strings.TrimSpace(strings.Split(rest, "]")[0])
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != full {
		t.Fatalf("temp file content mismatch: got %d bytes want %d", len(data), len(full))
	}
	_ = os.Remove(path)
}
