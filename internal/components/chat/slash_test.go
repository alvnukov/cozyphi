package chat

import (
	"strings"
	"testing"
)

func TestActiveSlash(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		cursor int
		ok     bool
		query  string
	}{
		{"empty", "", 0, false, ""},
		{"slash alone", "/", 1, true, ""},
		{"prefix", "/resu", 5, true, "resu"},
		{"mid prefix", "/resume", 4, true, "res"},
		{"cursor at start", "/sessions", 0, false, ""},
		{"after space arg", "/resume abc", 11, false, ""},
		{"in cmd before space", "/resume abc", 7, true, "resume"},
		{"not at start", "hi /sessions", 12, false, ""},
		{"plain text", "hello", 5, false, ""},
		{"at mention", "@file", 5, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, start, end, ok := ActiveSlash(tt.value, tt.cursor)
			if ok != tt.ok {
				t.Fatalf("ok=%v want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if q != tt.query {
				t.Fatalf("query=%q want %q", q, tt.query)
			}
			if start != 0 || end != tt.cursor {
				t.Fatalf("range=[%d,%d) want [0,%d)", start, end, tt.cursor)
			}
			if !strings.HasPrefix(tt.value, "/") {
				t.Fatal("expected slash prefix")
			}
		})
	}
}
