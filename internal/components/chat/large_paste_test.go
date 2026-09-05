package chat

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui/input"

	"github.com/alvnukov/cozyphi/internal/components"
)

func TestChatInputLargeTerminalPasteSubmitsIntact(t *testing.T) {
	c := &ChatInput{MinBodyRows: 1, MaxBodyRows: 8}
	var submitted []string
	c.OnSubmit = func(text string) { submitted = append(submitted, text) }
	p := input.NewParser()
	feed := func(raw string) {
		for _, event := range p.Feed([]byte(raw)) {
			c.Handle(&components.EventContext{}, event)
		}
	}
	text := strings.Repeat("a", (1<<20)+4096) + "\nхвост🙂"
	feed("\x1b[200~")
	for offset := 0; offset < len(text); offset += 4096 {
		feed(text[offset:min(offset+4096, len(text))])
	}
	feed("\x1b[20")
	feed("1~")
	if len(submitted) != 0 {
		t.Fatalf("paste submitted %d times before Enter", len(submitted))
	}
	if c.Value != text {
		t.Fatalf("draft corrupted: got %d bytes, want %d", len(c.Value), len(text))
	}
	feed("\r")
	if len(submitted) != 1 || submitted[0] != text {
		t.Fatalf("Enter must submit the entire draft once; got %d submissions", len(submitted))
	}
}
