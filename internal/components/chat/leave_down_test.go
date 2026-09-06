package chat

import (
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/history"
)

// TestLeaveDownOnlyAtTheEndWithNothingLeftToRecall pins the seam the agent
// panel enters through: Down leaves the composer only once the caret has run
// out of text below it and the history has no later entry to bring back.
func TestLeaveDownOnlyAtTheEndWithNothingLeftToRecall(t *testing.T) {
	left := 0
	c := &ChatInput{MinBodyRows: 3, Value: "line1\nline2", Cursor: 2}
	c.OnLeaveDown = func() bool { left++; return true }

	c.Handle(&components.EventContext{}, key(xui.KeyDown))
	if left != 0 {
		t.Fatalf("Down mid-text must move the caret, not leave: %d", left)
	}
	if c.Cursor != len("line1\nline2") {
		// moveVert(1) from line1 lands on line2 at the same column, then the
		// end of the text is one more Down away.
		c.Cursor = len(c.Value)
	}

	c.Handle(&components.EventContext{}, key(xui.KeyDown))
	if left != 1 {
		t.Fatalf("Down at the end must leave the composer, got %d", left)
	}
	if c.Value != "line1\nline2" {
		t.Fatalf("leaving must not touch the draft, got %q", c.Value)
	}
}

// TestLeaveDownYieldsToHistoryRecall: the panel never steals a Down the
// history still has a use for.
func TestLeaveDownYieldsToHistoryRecall(t *testing.T) {
	h := history.Open("")
	h.Append("older")
	left := 0
	c := &ChatInput{MinBodyRows: 3, History: h}
	c.OnLeaveDown = func() bool { left++; return true }

	c.Handle(&components.EventContext{}, key(xui.KeyUp)) // recall "older"
	if c.Value != "older" {
		t.Fatalf("setup: expected a recall, got %q", c.Value)
	}
	c.Handle(&components.EventContext{}, key(xui.KeyDown)) // restores the draft
	if left != 0 || c.Value != "" {
		t.Fatalf("Down must restore the draft first: left=%d value=%q", left, c.Value)
	}
	c.Handle(&components.EventContext{}, key(xui.KeyDown))
	if left != 1 {
		t.Fatalf("an empty composer with nothing left to recall must leave, got %d", left)
	}
}

// TestLeaveDownDeclinedKeepsCaretMotion: a shell with nowhere to go returns
// false and the composer behaves exactly as it did before the seam existed.
func TestLeaveDownDeclinedKeepsCaretMotion(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "one", Cursor: 3}
	c.OnLeaveDown = func() bool { return false }
	c.Handle(&components.EventContext{}, key(xui.KeyDown))
	if c.Value != "one" || c.Cursor != 3 {
		t.Fatalf("declined leave changed the editor: %q@%d", c.Value, c.Cursor)
	}
}
