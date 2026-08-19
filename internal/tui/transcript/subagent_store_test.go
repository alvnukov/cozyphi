package transcript_test

import (
	"testing"

	"github.com/pulseaiclub/phi/internal/components/status"
	"github.com/pulseaiclub/phi/internal/job"
	"github.com/pulseaiclub/phi/internal/tools"
	"github.com/pulseaiclub/phi/internal/tui/transcript"
)

func TestSubagentStoreProgressAndResult(t *testing.T) {
	s := transcript.NewSubagentStore()
	s.Bind("job1", "parent1")
	s.ApplyProgress(job.Progress{
		JobID:           "job1",
		ParentToolUseID: "parent1",
		ToolUseID:       "c1",
		Name:            "read",
		Status:          "in-progress",
		Detail:          "a.go",
	})
	s.ApplyProgress(job.Progress{
		JobID:           "job1",
		ParentToolUseID: "parent1",
		ToolUseID:       "c1",
		Name:            "read",
		Status:          "done",
		Detail:          "a.go",
	})
	s.ApplyProgress(job.Progress{
		JobID:           "job1",
		ParentToolUseID: "parent1",
		ToolUseID:       "c2",
		Name:            "bash",
		Status:          "done",
		Detail:          "test",
	})

	kids := s.Children("parent1")
	if len(kids) != 2 {
		t.Fatalf("len=%d", len(kids))
	}
	if kids[0].Status != status.ToolDone || kids[0].Name != "read" {
		t.Fatalf("%+v", kids[0])
	}
	byJob := s.ChildrenByJob("job1")
	if len(byJob) != 2 {
		t.Fatalf("byJob len=%d", len(byJob))
	}

	s.ApplyResult("parent1", tools.ParseAgentResult(`{
		"job_id":"job1","status":"completed","summary":"## Ok"
	}`))
	// Summary is stored; Children unchanged.
	if len(s.Children("parent1")) != 2 {
		t.Fatal("children cleared")
	}
}
