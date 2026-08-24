package transcript

import (
	"testing"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/block"
	"github.com/pulseaiclub/phi/internal/components/status"
	"github.com/pulseaiclub/phi/internal/session"
)

func TestTranscriptPane_ApplySessionAndSync(t *testing.T) {
	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	pane := NewTranscriptPane(th, spin, "Phi test")

	pane.ApplySession(session.UserAppend{Text: "hello"})
	pane.Sync()

	if pane.IsEmpty() {
		t.Fatal("expected transcript entries after user append")
	}
	if len(pane.Snapshot().Messages) != 1 {
		t.Fatalf("snap messages = %d, want 1", len(pane.Snapshot().Messages))
	}
}

func TestTranscriptPane_IsStreaming(t *testing.T) {
	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	pane := NewTranscriptPane(th, spin, "Phi test")

	if pane.IsStreaming() {
		t.Fatal("empty pane should not stream")
	}

	pane.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
		ID:    "a1",
		State: session.StateStreaming,
	}})
	if !pane.IsStreaming() {
		t.Fatal("expected streaming after assistant StateStreaming")
	}

	pane.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
		ID:    "a1",
		State: session.StateComplete,
	}})
	if pane.IsStreaming() {
		t.Fatal("expected idle after StreamEnd")
	}
}

func TestTranscriptPane_LoadReplayClearsWidgets(t *testing.T) {
	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	pane := NewTranscriptPane(th, spin, "Phi test")

	pane.ApplySession(session.UserAppend{Text: "x"})
	pane.Sync()
	if pane.IsEmpty() {
		t.Fatal("setup: expected entries")
	}

	pane.LoadReplay(session.Snapshot{})
	pane.Sync()
	if !pane.IsEmpty() {
		t.Fatal("LoadReplay should clear visible entries until snap has items")
	}
}

func TestTranscriptPaneTailSyncUpdatesVisibleAssistant(t *testing.T) {
	pane := NewTranscriptPane(components.DefaultTheme(), nil, "test")
	pane.ApplySession(streamingUpdate(0))
	pane.Sync()

	pane.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
		ID:    "assistant-current",
		State: session.StateStreaming,
		Content: []session.ContentBlock{
			{Type: session.BlockText, Text: "updated answer"},
		},
	}})
	pane.Sync()

	if len(pane.list.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(pane.list.Entries))
	}
	answer, ok := pane.list.Entries[0].(*block.AssistantBlock)
	if !ok || answer.Text != "updated answer" {
		t.Fatalf("assistant = %#v, want updated answer", pane.list.Entries[0])
	}
}

func TestTranscriptPaneTailSyncFallsBackWhenRowsChange(t *testing.T) {
	pane := NewTranscriptPane(components.DefaultTheme(), nil, "test")
	pane.ApplySession(streamingUpdate(0))
	pane.Sync()

	pane.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
		ID:    "assistant-current",
		State: session.StateStreaming,
		Content: []session.ContentBlock{
			{Type: session.BlockText, Text: "before tool"},
			{Type: session.BlockToolUse, ID: "tool-1", Name: "read", Input: "file.go"},
		},
	}})
	pane.Sync()

	if len(pane.list.Entries) != 2 {
		t.Fatalf("entries = %d, want assistant and tool rows", len(pane.list.Entries))
	}
	if _, ok := pane.list.Entries[1].(*block.ToolBlock); !ok {
		t.Fatalf("tail row = %T, want *block.ToolBlock", pane.list.Entries[1])
	}
}
