package chat

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

// TestChatInputQueuedStripIsBounded: MinHeight is a hard floor for the layout
// arbiter, so a strip that grew one row per queued prompt would push the input
// itself off a short screen — a 24-row terminal lost it at 14 queued prompts.
// Past the cap the remainder collapses into one counter row.
func TestChatInputQueuedStripIsBounded(t *testing.T) {
	bare := &ChatInput{MinBodyRows: 3}
	base := bare.MinHeight()

	c := &ChatInput{MinBodyRows: 3, Theme: components.DefaultTheme()}
	for i := range 40 {
		c.Queued = append(c.Queued, strings.Repeat("q", i+1))
	}
	if got, want := c.MinHeight(), base+maxQueuedRows; got != want {
		t.Fatalf("min height with 40 queued = %d, want %d", got, want)
	}

	s := c.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 40}, Method: xui.WidthUnicode})
	var rows []string
	for y := range s.Size.Height {
		if row := rowString(s, y); strings.Contains(row, "queued:") {
			rows = append(rows, row)
		}
	}
	if len(rows) != maxQueuedRows {
		t.Fatalf("queued rows = %d, want %d", len(rows), maxQueuedRows)
	}
	if last := rows[len(rows)-1]; !strings.Contains(last, "37 more") {
		t.Fatalf("last row = %q, want the count of the prompts the strip did not draw", last)
	}
}

// TestChatInputQueuedStripDrawsEveryPromptUnderTheCap: below the cap there is
// nothing to summarize, so every prompt keeps its own row and the recall hint.
func TestChatInputQueuedStripDrawsEveryPromptUnderTheCap(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Theme: components.DefaultTheme(), Queued: []string{"first", "second"}}

	s := c.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 20}, Method: xui.WidthUnicode})
	var joined []string
	for y := range s.Size.Height {
		if row := rowString(s, y); strings.Contains(row, "queued:") {
			joined = append(joined, row)
		}
	}
	if len(joined) != 2 {
		t.Fatalf("queued rows = %d, want 2", len(joined))
	}
	for i, want := range []string{"first", "second"} {
		if !strings.Contains(joined[i], want) || !strings.Contains(joined[i], "Esc to recall") {
			t.Fatalf("row %d = %q, want %q with the recall hint", i, joined[i], want)
		}
	}
}
