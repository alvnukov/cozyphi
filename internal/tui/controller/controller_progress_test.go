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
