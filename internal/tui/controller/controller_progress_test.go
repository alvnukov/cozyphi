package controller

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/job"
)

func TestShouldPublishJobProgressDedup(t *testing.T) {
	c := &Controller{}
	p := job.Progress{
		JobID:     "j",
		ToolUseID: "t1",
		Name:      "read",
		Status:    "in-progress",
		Detail:    "a.go",
	}
	if !c.shouldPublishJobProgress(p) {
		t.Fatal("first should publish")
	}
	if c.shouldPublishJobProgress(p) {
		t.Fatal("duplicate should drop")
	}
	p.Status = "done"
	if !c.shouldPublishJobProgress(p) {
		t.Fatal("status change should publish")
	}
	p2 := job.Progress{
		JobID:     "j",
		ToolUseID: "t2",
		Name:      "bash",
		Status:    "done",
		Detail:    "ls",
	}
	if !c.shouldPublishJobProgress(p2) {
		t.Fatal("new child should publish")
	}
}

// TestTerminalJobProgressEvictsItsKey pins that a closed child slot frees its
// dedupe key: the map tracks live slots, not every slot the session ever had.
func TestTerminalJobProgressEvictsItsKey(t *testing.T) {
	c := &Controller{}
	done := job.Progress{JobID: "j", ToolUseID: "t1", Name: "read", Status: "done", Detail: "a.go"}
	if !c.shouldPublishJobProgress(done) {
		t.Fatal("terminal should publish")
	}
	if _, held := c.lastJobProgress.Load("j\x00t1"); held {
		t.Fatal("a closed slot must not hold its dedupe key")
	}

	// The fallback keys (progress without a ToolUseID) are evicted too.
	failed := job.Progress{JobID: "j", Name: "spawn", Status: "error", Detail: "x"}
	if !c.shouldPublishJobProgress(failed) {
		t.Fatal("terminal should publish")
	}
	if _, held := c.lastJobProgress.Load("j\x00spawn\x00x"); held {
		t.Fatal("a failed slot must not hold its dedupe key")
	}
}
