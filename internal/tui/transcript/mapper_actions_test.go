package transcript_test

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

// anchors records the entry id every action was handed.
type anchors struct{ rewind, fork, aside []string }

func (a *anchors) wire(m *transcript.Mapper) {
	m.SetMessageActions(
		func(id string) { a.rewind = append(a.rewind, id) },
		func(id string) { a.fork = append(a.fork, id) },
		func(id string) { a.aside = append(a.aside, id) },
	)
}

// clickStrip draws the widget wide, then presses the button whose label
// contains want. It reports whether a button was found at all.
func clickStrip(t *testing.T, w components.Widget, want string) bool {
	t.Helper()
	ctx := components.DrawContext{
		Max:    components.Size{Width: 90, Height: 20},
		Method: xui.WidthUnicode,
	}
	s := w.Draw(ctx)
	for y, row := range strings.Split(components.SurfaceText(s), "\n") {
		before, _, found := strings.Cut(row, want)
		if !found {
			continue
		}
		x := xui.StringWidth(before, xui.WidthUnicode)
		w.Handle(&components.EventContext{}, xui.MouseEvent{
			X: x, Y: y, Button: xui.MouseLeft, Action: xui.MousePress,
		})
		return true
	}
	return false
}

// The anchor a click carries is the session entry, not the row: an assistant
// message reaches the feed cut into text segments, and every segment's
// actions must still name the message the log knows.
func TestMessageActionsAnchorOnTheSessionEntry(t *testing.T) {
	var rec anchors
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	rec.wire(m)

	entries, _, _ := m.Sync(nil, nil, session.Snapshot{Messages: []session.Message{
		{ID: "u1", Role: session.RoleUser, Text: "do the thing"},
		{
			ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
			Model: "model", StopReason: session.StopEndTurn,
			Content: []session.ContentBlock{{Type: session.BlockText, Text: "done"}},
		},
	}})

	if !clickStrip(t, entries[0], "rewind") {
		t.Fatal("the prompt has no rewind button")
	}
	if len(rec.rewind) != 1 || rec.rewind[0] != "u1" {
		t.Fatalf("rewind anchors = %v, want the prompt entry", rec.rewind)
	}

	reply := entries[len(entries)-1]
	if _, ok := reply.(*block.AssistantBlock); !ok {
		t.Fatalf("last entry = %T", reply)
	}
	if !clickStrip(t, reply, "fork") {
		t.Fatal("the closing reply has no fork button")
	}
	if len(rec.fork) != 1 || rec.fork[0] != "a1" {
		t.Fatalf("fork anchors = %v, want the message id without its text segment", rec.fork)
	}
}

// A failed turn leaves a row the session log never received. There is
// nothing to rewind or fork to, so the row carries no strip.
func TestErrorRowCarriesNoActions(t *testing.T) {
	var rec anchors
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	rec.wire(m)

	entries, _, _ := m.Sync(nil, nil, session.Snapshot{Messages: []session.Message{{
		ID: "assistant-error-1730000000", Role: session.RoleAssistant,
		State:   session.StateError,
		Content: []session.ContentBlock{{Type: session.BlockText, Text: "model refused"}},
	}}})

	for _, w := range entries {
		for _, label := range []string{"rewind", "fork", "btw"} {
			if clickStrip(t, w, label) {
				t.Fatalf("the error row offers %q", label)
			}
		}
	}
}

// Only a round's closing reply may cut the context. A reply followed by a
// tool call keeps the side question and loses the other two.
func TestMidTurnReplyOffersOnlyTheSideQuestion(t *testing.T) {
	var rec anchors
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	rec.wire(m)

	entries, _, _ := m.Sync(nil, nil, session.Snapshot{
		Messages: []session.Message{{
			ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
			Model: "model", StopReason: session.StopToolUse,
			Content: []session.ContentBlock{
				{Type: session.BlockText, Text: "reading the file first"},
				{Type: session.BlockToolUse, ID: "t1", Name: "read", Input: "a.go"},
			},
		}},
		Tools: map[string]session.ToolRun{
			"t1": {ToolUseID: "t1", Name: "read", Status: session.ToolDone},
		},
	})

	reply, ok := entries[0].(*block.AssistantBlock)
	if !ok {
		t.Fatalf("entries[0] = %T", entries[0])
	}
	if clickStrip(t, reply, "rewind") || clickStrip(t, reply, "fork") {
		t.Fatal("a reply with an open tool call offers a cut of the context")
	}
	if !clickStrip(t, reply, "btw") {
		t.Fatal("a mid-turn reply lost the side question")
	}
	if len(rec.aside) != 1 || rec.aside[0] != "a1" {
		t.Fatalf("aside anchors = %v", rec.aside)
	}
}

// While a turn is in flight the strips refuse and say so, and they come back
// as soon as the turn ends.
func TestActionsRefuseWhileTheTurnRuns(t *testing.T) {
	var rec anchors
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	rec.wire(m)

	streaming := session.Snapshot{Messages: []session.Message{{
		ID: "u1", Role: session.RoleUser, Text: "go",
	}, {
		ID: "a1", Role: session.RoleAssistant, State: session.StateStreaming,
		Model: "model", Content: []session.ContentBlock{
			{Type: session.BlockText, Text: "working"},
		},
	}}}
	entries, ids, _ := m.Sync(nil, nil, streaming)
	prompt, ok := entries[0].(*block.UserBlock)
	if !ok {
		t.Fatalf("entries[0] = %T", entries[0])
	}
	if !clickStrip(t, prompt, "rewind") {
		t.Fatal("the prompt lost its strip while the turn runs")
	}
	if len(rec.rewind) != 0 {
		t.Fatalf("a click acted mid-turn: %v", rec.rewind)
	}
	if hint, ok := hintOnStrip(t, prompt, "rewind"); !ok || hint == "" {
		t.Fatalf("a refusing button explains nothing: %q", hint)
	}

	done := session.Snapshot{Messages: []session.Message{streaming.Messages[0], {
		ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
		Model: "model", StopReason: session.StopEndTurn,
		Content: []session.ContentBlock{{Type: session.BlockText, Text: "working"}},
	}}}
	entries, _, _ = m.Sync(entries, ids, done)
	prompt, ok = entries[0].(*block.UserBlock)
	if !ok {
		t.Fatalf("entries[0] = %T", entries[0])
	}
	if !clickStrip(t, prompt, "rewind") || len(rec.rewind) != 1 {
		t.Fatalf("the strip did not come back after the turn: %v", rec.rewind)
	}
}

// hintOnStrip returns the tooltip of the button whose label contains want.
func hintOnStrip(t *testing.T, w components.Widget, want string) (string, bool) {
	t.Helper()
	s := w.Draw(components.DrawContext{
		Max:    components.Size{Width: 90, Height: 20},
		Method: xui.WidthUnicode,
	})
	tipper, ok := w.(components.HoverTooltiper)
	if !ok {
		return "", false
	}
	for y, row := range strings.Split(components.SurfaceText(s), "\n") {
		before, _, found := strings.Cut(row, want)
		if !found {
			continue
		}
		return tipper.HoverTooltip(xui.StringWidth(before, xui.WidthUnicode), y)
	}
	return "", false
}

// A round the token limit cut short closes with a warning row named after
// the message it came from. That message is in the log, so the row anchors
// on it rather than on the id the projection invented for the warning.
func TestTruncatedRowAnchorsOnItsMessage(t *testing.T) {
	var rec anchors
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	rec.wire(m)

	entries, _, _ := m.Sync(nil, nil, session.Snapshot{Messages: []session.Message{{
		ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
		Model: "model", StopReason: session.StopMaxTokens,
		Content: []session.ContentBlock{
			{Type: session.BlockThinking, Text: "reasoning that ate the budget"},
		},
	}}})

	reply := entries[len(entries)-1]
	if _, ok := reply.(*block.AssistantBlock); !ok {
		t.Fatalf("last entry = %T", reply)
	}
	if !clickStrip(t, reply, "rewind") {
		t.Fatal("the truncation warning has no rewind button")
	}
	if len(rec.rewind) != 1 || rec.rewind[0] != "a1" {
		t.Fatalf("rewind anchors = %v, want the message the round was cut from", rec.rewind)
	}
}

// A row no message stands behind offers nothing, whoever invented it.
func TestRowWithoutAMessageCarriesNoActions(t *testing.T) {
	var rec anchors
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	rec.wire(m)

	entries, _, _ := m.Sync(nil, nil, session.Snapshot{Messages: []session.Message{{
		ID: "follow-up-error-1730000000", Role: session.RoleAssistant,
		State:   session.StateError,
		Content: []session.ContentBlock{{Type: session.BlockText, Text: "follow-up failed"}},
	}}})

	for _, w := range entries {
		for _, label := range []string{"rewind", "fork", "btw"} {
			if clickStrip(t, w, label) {
				t.Fatalf("a row with no session entry offers %q", label)
			}
		}
	}
}

// The reply widget is patched in place while the round streams, so the same
// block is drawn again and again with a growing body and then with a footer
// row. The strip has to travel with it and leave nothing behind.
func TestStripTravelsWithAPatchedReply(t *testing.T) {
	var rec anchors
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	rec.wire(m)

	step := func(entries []components.Widget, ids []string, msg session.Message) ([]components.Widget, []string) {
		t.Helper()
		snap := session.Snapshot{
			Messages: []session.Message{
				{ID: "u1", Role: session.RoleUser, Text: "go"},
				msg,
			},
			Tools: map[string]session.ToolRun{
				"t1": {ToolUseID: "t1", Name: "read", Status: session.ToolDone},
			},
		}
		entries, ids, _ = m.Sync(entries, ids, snap)
		reply, ok := entries[1].(*block.AssistantBlock)
		if !ok {
			t.Fatalf("last entry = %T", entries[len(entries)-1])
		}
		s := reply.Draw(components.DrawContext{
			Max:    components.Size{Width: 90, Height: 30},
			Method: xui.WidthUnicode,
		})
		rows := 0
		for row := range strings.SplitSeq(components.SurfaceText(s), "\n") {
			if strings.Contains(row, "btw") {
				rows++
			}
		}
		if rows != 1 {
			t.Fatalf("the strip is on %d rows of the patched reply:\n%s", rows, components.SurfaceText(s))
		}
		return entries, ids
	}

	// A round still holding a tool call carries no footer row, so the strip
	// rides the reply's last line of text and moves as the text grows.
	midTurn := func(text string) session.Message {
		return session.Message{
			ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
			Model: "model", StopReason: session.StopToolUse,
			Content: []session.ContentBlock{
				{Type: session.BlockText, Text: text},
				{Type: session.BlockToolUse, ID: "t1", Name: "read", Input: "a.go"},
			},
		}
	}
	entries, ids := step(nil, nil, midTurn("first"))
	entries, ids = step(entries, ids, midTurn("first\n\nsecond\n\nthird"))
	// Closing the round adds the footer, and the strip moves onto it.
	entries, _ = step(entries, ids, session.Message{
		ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
		Model: "model", StopReason: session.StopEndTurn,
		Content: []session.ContentBlock{
			{Type: session.BlockText, Text: "first\n\nsecond\n\nthird"},
		},
	})

	reply := entries[1]
	if !clickStrip(t, reply, "rewind") {
		t.Fatal("the closed round has no rewind button")
	}
	if len(rec.rewind) != 1 || rec.rewind[0] != "a1" {
		t.Fatalf("rewind anchors = %v", rec.rewind)
	}
}
