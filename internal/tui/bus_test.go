package tui_test

import (
	"testing"

	"github.com/pulseaiclub/phi/internal/job"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tui"
)

func TestBusOnWakeOnceUntilDrain(t *testing.T) {
	var wakes int
	b := tui.NewBus(func() { wakes++ })
	b.Publish(tui.JobProgressMsg{Progress: job.Progress{JobID: "j", ToolUseID: "t1", Status: "in-progress"}})
	b.Publish(tui.JobProgressMsg{Progress: job.Progress{JobID: "j", ToolUseID: "t1", Status: "done"}})
	b.Publish(tui.JobProgressMsg{Progress: job.Progress{JobID: "j", ToolUseID: "t2", Status: "done"}})
	if wakes != 1 {
		t.Fatalf("wakes=%d want 1 before Drain", wakes)
	}
	batch := b.Drain()
	if len(batch) != 2 {
		t.Fatalf("len=%d want 2", len(batch))
	}
	b.Publish(tui.JobProgressMsg{Progress: job.Progress{JobID: "j", ToolUseID: "t3", Status: "done"}})
	if wakes != 2 {
		t.Fatalf("wakes=%d want 2 after re-arm", wakes)
	}
}

func TestBusCoalesceNonAdjacentAssistant(t *testing.T) {
	var wakes int
	b := tui.NewBus(func() { wakes++ })
	b.Publish(tui.SessionEventMsg{Event: session.AssistantMessageUpdate{Message: session.Message{
		ID: "a1", Text: "one", State: session.StateStreaming,
	}}})
	b.Publish(tui.JobProgressMsg{Progress: job.Progress{JobID: "j", ToolUseID: "t1", Status: "in-progress"}})
	b.Publish(tui.SessionEventMsg{Event: session.AssistantMessageUpdate{Message: session.Message{
		ID: "a1", Text: "two", State: session.StateStreaming,
	}}})
	if wakes != 1 {
		t.Fatalf("wakes=%d want 1", wakes)
	}
	batch := b.Drain()
	if len(batch) != 2 {
		t.Fatalf("len=%d want 2 (assistant coalesced across progress)", len(batch))
	}
	te := batch[0].(tui.SessionEventMsg)
	upd := te.Event.(session.AssistantMessageUpdate)
	if upd.Message.Text != "two" {
		t.Fatalf("text=%q want two", upd.Message.Text)
	}
}

func TestBusCoalesceJobProgressAcrossSession(t *testing.T) {
	b := tui.NewBus(nil)
	b.Publish(tui.JobProgressMsg{Progress: job.Progress{JobID: "j", ToolUseID: "t1", Status: "in-progress"}})
	b.Publish(tui.SessionEventMsg{Event: session.AssistantMessageUpdate{Message: session.Message{
		ID: "a1", Text: "x", State: session.StateStreaming,
	}}})
	b.Publish(tui.JobProgressMsg{Progress: job.Progress{JobID: "j", ToolUseID: "t1", Status: "done"}})
	batch := b.Drain()
	if len(batch) != 2 {
		t.Fatalf("len=%d want 2", len(batch))
	}
	jp := batch[0].(tui.JobProgressMsg)
	if jp.Progress.Status != "done" {
		t.Fatalf("status=%q", jp.Progress.Status)
	}
}
