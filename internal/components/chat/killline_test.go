package chat

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/pulseaiclub/xui/input"

	"github.com/alvnukov/cozyphi/internal/components"
)

func ctrlU() xui.KeyEvent {
	return xui.KeyEvent{Code: xui.KeyRune, Rune: 'u', Mods: xui.ModCtrl, Press: true}
}

// The everyday case the chord exists for: one line of draft, caret at its
// end, Ctrl+U empties the composer.
func TestCtrlUClearsASingleLineDraft(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "throw this away", Cursor: 15}
	ctx := &components.EventContext{}
	c.Handle(ctx, ctrlU())
	if c.Value != "" || c.Cursor != 0 {
		t.Fatalf("value=%q cursor=%d, want an empty composer", c.Value, c.Cursor)
	}
	if !ctx.Consume {
		t.Fatal("Ctrl+U must be consumed by the composer")
	}
}

// It is a line kill, not a buffer kill: the rest of a multi-line draft, and
// anything after the caret on its own line, must survive.
func TestCtrlUKeepsTheRestOfTheDraft(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "keep me\nkill thiskeep too", Cursor: len("keep me\nkill this")}
	c.Handle(&components.EventContext{}, ctrlU())
	if want := "keep me\nkeep too"; c.Value != want {
		t.Fatalf("value = %q, want %q", c.Value, want)
	}
	if c.Cursor != len("keep me\n") {
		t.Fatalf("cursor = %d, want the line start %d", c.Cursor, len("keep me\n"))
	}
}

// At the start of a line there is nothing on it to discard, so the draft
// stays whole rather than eating the newline before it.
func TestCtrlUAtLineStartChangesNothing(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "first\nsecond", Cursor: 6}
	changes := 0
	c.OnChange = func(string) { changes++ }
	c.Handle(&components.EventContext{}, ctrlU())
	if c.Value != "first\nsecond" || c.Cursor != 6 {
		t.Fatalf("value=%q cursor=%d, want the draft untouched", c.Value, c.Cursor)
	}
	if changes != 0 {
		t.Fatalf("a no-op kill must not report a change, got %d", changes)
	}
}

// A selection is what the user can see, so it wins over the line, the same
// way Backspace treats it.
func TestCtrlUDeletesTheSelectionWhenThereIsOne(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "alpha beta gamma", Cursor: 0}
	c.SetSelection(6, 11)
	c.Handle(&components.EventContext{}, ctrlU())
	if want := "alpha gamma"; c.Value != want {
		t.Fatalf("value = %q, want %q", c.Value, want)
	}
	if c.HasSelection() {
		t.Fatal("the selection must be gone after the kill")
	}
}

// End to end from the wire: a terminal sends Ctrl+U as the raw NAK byte.
func TestCtrlURawByteReachesTheKill(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "typed by hand", Cursor: 13}
	drawEditor(c, 60)
	p := input.NewParser()
	for _, ev := range p.Feed([]byte{0x15}) {
		if ke, ok := ev.(xui.KeyEvent); ok {
			c.Handle(&components.EventContext{}, ke)
		}
	}
	if c.Value != "" {
		t.Fatalf("value = %q, want the line discarded", c.Value)
	}
}
