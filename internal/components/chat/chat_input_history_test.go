package chat

import (
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/editmode"
	"github.com/alvnukov/cozyphi/internal/history"
)

// key builds a press event for ChatInput.Handle.
func key(code xui.KeyCode) xui.KeyEvent {
	return xui.KeyEvent{Code: code, Press: true}
}

// TestChatInputHistoryRecall walks the prompt history with Up/Down the way
// opencode's prompt.history.previous/next commands do: Up at the text origin
// recalls the newest submission, Down at the end walks back and finally
// restores the draft.
func TestChatInputHistoryRecall(t *testing.T) {
	h := history.Open("")
	h.Append("first")
	h.Append("second")
	c := &ChatInput{MinBodyRows: 3, History: h}

	c.Handle(&components.EventContext{}, key(xui.KeyUp))
	if c.Value != "second" || c.Cursor != len("second") {
		t.Fatalf("after Up: value=%q cursor=%d", c.Value, c.Cursor)
	}

	c.Handle(&components.EventContext{}, key(xui.KeyUp))
	if c.Value != "first" {
		t.Fatalf("second Up must walk older, got %q", c.Value)
	}

	c.Handle(&components.EventContext{}, key(xui.KeyDown))
	if c.Value != "second" {
		t.Fatalf("Down must walk newer, got %q", c.Value)
	}

	c.Handle(&components.EventContext{}, key(xui.KeyDown))
	if c.Value != "" || c.Cursor != 0 {
		t.Fatalf("Down past newest must restore the draft, got %q@%d", c.Value, c.Cursor)
	}

	// One more Down at the draft slot changes nothing.
	c.Handle(&components.EventContext{}, key(xui.KeyDown))
	if c.Value != "" {
		t.Fatalf("Down at draft slot must be a no-op, got %q", c.Value)
	}
}

// TestChatInputHistoryNeedsEdgeCursor: history only claims Up at the text
// origin and Down at the end; anywhere else the keys keep moving the caret
// through multiline text.
func TestChatInputHistoryNeedsEdgeCursor(t *testing.T) {
	h := history.Open("")
	h.Append("older")
	c := &ChatInput{MinBodyRows: 3, History: h, Value: "line1\nline2", Cursor: 6}

	// Up mid-text moves the caret to the previous line, not history.
	c.Handle(&components.EventContext{}, key(xui.KeyUp))
	if c.Cursor != 0 || c.Value != "line1\nline2" {
		t.Fatalf("Up mid-text: cursor=%d value=%q", c.Cursor, c.Value)
	}

	// Up at the origin recalls — even over a typed draft…
	c.Handle(&components.EventContext{}, key(xui.KeyUp))
	if c.Value != "older" || c.Cursor != len("older") {
		t.Fatalf("Up at origin must recall, got %q@%d", c.Value, c.Cursor)
	}

	// …and Down at the end brings the captured draft back.
	c.Handle(&components.EventContext{}, key(xui.KeyDown))
	if c.Value != "line1\nline2" || c.Cursor != len("line1\nline2") {
		t.Fatalf("Down must restore the draft, got %q@%d", c.Value, c.Cursor)
	}

	// Down mid-text moves the caret to the next line, not history.
	c.Cursor = 0
	c.Handle(&components.EventContext{}, key(xui.KeyDown))
	if c.Cursor != 6 || c.Value != "line1\nline2" {
		t.Fatalf("Down mid-text: cursor=%d value=%q", c.Cursor, c.Value)
	}
}

// TestChatInputHistoryNilIsInert: without a store, Up/Down keep the plain
// caret behavior.
func TestChatInputHistoryNilIsInert(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "line1\nline2", Cursor: 6}
	c.Handle(&components.EventContext{}, key(xui.KeyUp))
	if c.Cursor != 0 || c.Value != "line1\nline2" {
		t.Fatalf("Up without history: cursor=%d value=%q", c.Cursor, c.Value)
	}
}

// TestChatInputHistoryKeepsPickersShut: recalling a "/command" must not wake
// the slash picker. A recalled entry is a finished prompt, and an open picker
// would grab the next Up/Down for its own navigation, ending the walk.
func TestChatInputHistoryKeepsPickersShut(t *testing.T) {
	h := history.Open("")
	h.Append("/clear")
	c := &ChatInput{MinBodyRows: 3, History: h}
	slashActive, mentionActive, argActive := false, false, false
	c.OnSlashChange = func(active bool, _ string) { slashActive = active }
	c.OnMentionChange = func(active bool, _ string) { mentionActive = active }
	c.OnSlashArgChange = func(active bool, _ string, _ []string, _ string) { argActive = active }

	c.Handle(&components.EventContext{}, key(xui.KeyUp))

	if c.Value != "/clear" {
		t.Fatalf("Up must recall the slash entry, got %q", c.Value)
	}
	if slashActive || mentionActive || argActive {
		t.Fatalf("recall opened a completer: slash=%v mention=%v arg=%v", slashActive, mentionActive, argActive)
	}
}

// TestChatInputHistoryReadlineKeepsPickersShut: Ctrl+P shares the recall
// semantics of Up, down to leaving the pickers alone.
func TestChatInputHistoryReadlineKeepsPickersShut(t *testing.T) {
	h := history.Open("")
	h.Append("/clear")
	c := &ChatInput{MinBodyRows: 3, History: h}
	c.SetEditingMode(editmode.Readline)
	slashActive := false
	c.OnSlashChange = func(active bool, _ string) { slashActive = active }

	editKey(c, 'p', xui.ModCtrl)

	if c.Value != "/clear" {
		t.Fatalf("Ctrl+P must recall the slash entry, got %q", c.Value)
	}
	if slashActive {
		t.Fatal("Ctrl+P recall opened the slash picker")
	}
}
