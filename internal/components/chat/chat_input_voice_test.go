package chat

import (
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

// handle runs one key through the input and reports whether it was consumed —
// what the dispatcher reads to decide if the pane ladder still gets the event.
func handle(c *ChatInput, ev xui.KeyEvent) bool {
	ctx := &components.EventContext{}
	c.Handle(ctx, ev)
	return ctx.Consume
}

// TestVoiceModeDefersSpaceAndEnter: while the composer is in the voice dialog
// mode the focused input leaves a plain Space and a plain Enter unconsumed, so
// the pane's mode handling sees them; everything else still types.
func TestVoiceModeDefersSpaceAndEnter(t *testing.T) {
	submits := 0
	c := &ChatInput{MinBodyRows: 3, VoiceMode: true, Value: "said"}
	c.Cursor = len(c.Value)
	c.OnSubmit = func(string) { submits++ }

	if handle(c, runeKey(' ')) {
		t.Fatal("a plain Space must be left for the voice mode")
	}
	if handle(c, key(xui.KeyEnter)) {
		t.Fatal("a plain Enter must be left for the voice mode")
	}
	if c.Value != "said" {
		t.Fatalf("value = %q, want the buffer untouched", c.Value)
	}
	if submits != 0 {
		t.Fatalf("submits = %d, want the send left to the voice mode", submits)
	}

	// Modified Enter still inserts a newline, and letters still type.
	if !handle(c, xui.KeyEvent{Code: xui.KeyEnter, Mods: xui.ModShift, Press: true}) {
		t.Fatal("Shift+Enter is still the input's newline")
	}
	if !handle(c, runeKey('x')) {
		t.Fatal("an ordinary rune still types")
	}
	if c.Value != "said\nx" {
		t.Fatalf("value = %q, want \"said\\nx\"", c.Value)
	}
}

// TestVoiceModeGivesSpaceBackToAnOpenPicker: the picker's query is typed in
// this buffer, so a space belongs to it even while the mode is on.
func TestVoiceModeGivesSpaceBackToAnOpenPicker(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, VoiceMode: true, SlashOpen: true}

	if !handle(c, runeKey(' ')) {
		t.Fatal("an open picker keeps the space")
	}
	if c.Value != " " {
		t.Fatalf("value = %q, want a typed space", c.Value)
	}
}

// TestWithoutVoiceModeSpaceAndEnterAreTheInputs: the deferral is the flag's
// doing and nothing else.
func TestWithoutVoiceModeSpaceAndEnterAreTheInputs(t *testing.T) {
	var submitted string
	c := &ChatInput{MinBodyRows: 3}
	c.OnSubmit = func(text string) { submitted = text }

	if !handle(c, runeKey(' ')) {
		t.Fatal("Space types without the mode")
	}
	if c.Value != " " {
		t.Fatalf("value = %q, want a typed space", c.Value)
	}
	if !handle(c, key(xui.KeyEnter)) {
		t.Fatal("Enter submits without the mode")
	}
	if submitted != " " {
		t.Fatalf("submitted = %q", submitted)
	}
}
