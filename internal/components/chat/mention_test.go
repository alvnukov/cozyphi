package chat

import "testing"

func TestActiveMention(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		cursor int
		ok     bool
		query  string
		start  int
	}{
		{"empty", "", 0, false, "", 0},
		{"at alone", "@", 1, true, "", 0},
		{"at query", "@man", 4, true, "man", 0},
		{"mid query", "@manager", 4, true, "man", 0},
		{"after space", "see @go", 7, true, "go", 4},
		{"email", "a@b.com", 7, false, "", 0},
		{"after newline", "hi\n@x", 5, true, "x", 3},
		{"after paren", "(@file", 6, true, "file", 1},
		{"space ends", "@a b", 2, true, "a", 0},
		{"cursor before at", "@a", 0, false, "", 0},
		{"closed by space at cursor", "look @path ", 11, false, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, start, end, ok := ActiveMention(tt.value, tt.cursor)
			if ok != tt.ok {
				t.Fatalf("ok=%v want %v (q=%q)", ok, tt.ok, q)
			}
			if !ok {
				return
			}
			if q != tt.query || start != tt.start || end != tt.cursor {
				t.Fatalf("q=%q start=%d end=%d want q=%q start=%d end=%d",
					q, start, end, tt.query, tt.start, tt.cursor)
			}
		})
	}
}

func TestReplaceRange(t *testing.T) {
	c := &ChatInput{Value: "see @man", Cursor: 8}
	c.ReplaceRange(4, 8, "@internal/session/manager.go")
	if c.Value != "see @internal/session/manager.go" {
		t.Fatalf("value=%q", c.Value)
	}
	if c.Cursor != len(c.Value) {
		t.Fatalf("cursor=%d", c.Cursor)
	}
}
