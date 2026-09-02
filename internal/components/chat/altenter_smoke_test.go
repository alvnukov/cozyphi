package chat

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/pulseaiclub/xui/input"

	"github.com/alvnukov/cozyphi/internal/components"
)

// End-to-end: legacy ESC CR from a terminal reaches ChatInput as
// Enter+ModAlt and inserts a newline without submitting.
func TestChatInputLegacyAltEnterEndToEnd(t *testing.T) {
	submitted := false
	c := &ChatInput{MinBodyRows: 3, Value: "hello world", Cursor: 5, OnSubmit: func(string) {
		submitted = true
	}}
	drawEditor(c, 60)
	p := input.NewParser()
	for _, ev := range p.Feed([]byte("\x1b\r")) {
		if ke, ok := ev.(xui.KeyEvent); ok {
			c.Handle(&components.EventContext{}, ke)
		}
	}
	if c.Value != "hello\n world" {
		t.Fatalf("value = %q", c.Value)
	}
	if submitted {
		t.Fatal("Alt+Enter must not submit")
	}
}
