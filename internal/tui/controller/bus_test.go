package controller_test

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

func TestBusOnWakeOnceUntilDrain(t *testing.T) {
	var wakes int
	b := controller.NewBus(func() { wakes++ })
	b.Publish(controller.JobProgressMsg{Progress: job.Progress{JobID: "j", ToolUseID: "t1", Status: "in-progress"}})
	b.Publish(controller.JobProgressMsg{Progress: job.Progress{JobID: "j", ToolUseID: "t1", Status: "done"}})
	b.Publish(controller.JobProgressMsg{Progress: job.Progress{JobID: "j", ToolUseID: "t2", Status: "done"}})
	if wakes != 1 {
		t.Fatalf("wakes=%d want 1 before Drain", wakes)
	}
	batch := b.Drain()
	if len(batch) != 2 {
		t.Fatalf("len=%d want 2", len(batch))
	}
	b.Publish(controller.JobProgressMsg{Progress: job.Progress{JobID: "j", ToolUseID: "t3", Status: "done"}})
	if wakes != 2 {
		t.Fatalf("wakes=%d want 2 after re-arm", wakes)
	}
}

func TestBusCoalesceNonAdjacentAssistant(t *testing.T) {
	var wakes int
	b := controller.NewBus(func() { wakes++ })
	b.Publish(controller.SessionEventMsg{Event: session.AssistantMessageUpdate{Message: session.Message{
		ID: "a1", Text: "one", State: session.StateStreaming,
	}}})
	b.Publish(controller.JobProgressMsg{Progress: job.Progress{JobID: "j", ToolUseID: "t1", Status: "in-progress"}})
	b.Publish(controller.SessionEventMsg{Event: session.AssistantMessageUpdate{Message: session.Message{
		ID: "a1", Text: "two", State: session.StateStreaming,
	}}})
	if wakes != 1 {
		t.Fatalf("wakes=%d want 1", wakes)
	}
	batch := b.Drain()
	if len(batch) != 2 {
		t.Fatalf("len=%d want 2 (assistant coalesced across progress)", len(batch))
	}
	te := batch[0].(controller.SessionEventMsg)
	upd := te.Event.(session.AssistantMessageUpdate)
	if upd.Message.Text != "two" {
		t.Fatalf("text=%q want two", upd.Message.Text)
	}
}

func TestBusKeepsDistinctAssistantRows(t *testing.T) {
	b := controller.NewBus(nil)
	b.Publish(controller.SessionEventMsg{Event: session.AssistantMessageUpdate{Message: session.Message{
		ID: "a1", Text: "round one", State: session.StateComplete,
	}}})
	b.Publish(controller.SessionEventMsg{Event: session.UserPromoted{ID: "u2"}})
	b.Publish(controller.SessionEventMsg{Event: session.AssistantMessageUpdate{Message: session.Message{
		ID: "a2", Text: "answered", State: session.StateStreaming,
	}}})
	batch := b.Drain()
	if len(batch) != 3 {
		t.Fatalf("len=%d want 3: a new round must never swallow the previous round's terminal event", len(batch))
	}
	first := batch[0].(controller.SessionEventMsg).Event.(session.AssistantMessageUpdate)
	if first.Message.ID != "a1" || first.Message.Text != "round one" {
		t.Fatalf("first=%+v: round one's terminal event must survive", first.Message)
	}
}

func TestBusCoalesceJobProgressAcrossSession(t *testing.T) {
	b := controller.NewBus(nil)
	b.Publish(controller.JobProgressMsg{Progress: job.Progress{JobID: "j", ToolUseID: "t1", Status: "in-progress"}})
	b.Publish(controller.SessionEventMsg{Event: session.AssistantMessageUpdate{Message: session.Message{
		ID: "a1", Text: "x", State: session.StateStreaming,
	}}})
	b.Publish(controller.JobProgressMsg{Progress: job.Progress{JobID: "j", ToolUseID: "t1", Status: "done"}})
	batch := b.Drain()
	if len(batch) != 2 {
		t.Fatalf("len=%d want 2", len(batch))
	}
	jp := batch[0].(controller.JobProgressMsg)
	if jp.Progress.Status != "done" {
		t.Fatalf("status=%q", jp.Progress.Status)
	}
}
