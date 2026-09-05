package chat

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui/input"

	"github.com/alvnukov/cozyphi/internal/components"
)

func TestChatRejectedPastePreservesWholeDraft(t *testing.T) {
	const draft = "before\nexisting🙂draft\nafter"
	c := &ChatInput{Value: draft, MinBodyRows: 1, MaxBodyRows: 8}
	var submitted []string
	c.OnSubmit = func(text string) { submitted = append(submitted, text) }
	p := input.NewParser()
	for _, raw := range []string{"\x1b[200~", strings.Repeat("a", input.MaxPasteBytes+1), "\r\n\x03\x1b[20", "1~"} {
		for _, event := range p.Feed([]byte(raw)) {
			c.Handle(&components.EventContext{}, event)
		}
	}
	if c.Value != draft || len(submitted) != 0 {
		t.Fatalf("rejection changed draft or submitted: draft=%q submissions=%d", c.Value, len(submitted))
	}
}
