package transcript_test

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
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

	run, ok := s.Run("parent1", "")
	if !ok || len(run.Children) != 2 {
		t.Fatalf("ok=%v len=%d", ok, len(run.Children))
	}
	if run.Children[0].Status != status.ToolDone || run.Children[0].Name != "read" {
		t.Fatalf("%+v", run.Children[0])
	}
	if !run.Running() {
		t.Fatal("a child with no outcome yet is still running")
	}
	byJob, ok := s.Run("", "job1")
	if !ok || len(byJob.Children) != 2 {
		t.Fatalf("byJob ok=%v len=%d", ok, len(byJob.Children))
	}

	s.ApplyResult("parent1", tools.ParseAgentResult(`{
		"job_id":"job1","status":"completed","summary":"## Ok"
	}`))
	// Summary and a stopped clock are stored; children unchanged.
	run, _ = s.Run("parent1", "")
	if len(run.Children) != 2 {
		t.Fatal("children cleared")
	}
	if run.Summary != "## Ok" || run.Status != job.StatusCompleted {
		t.Fatalf("result not recorded: %+v", run)
	}
	if run.Running() || run.Finished.IsZero() {
		t.Fatalf("clock must freeze at a terminal result: %+v", run)
	}
}

func TestSubagentStoreApplyOutcome(t *testing.T) {
	s := transcript.NewSubagentStore()
	s.Bind("job1", "parent1")

	outcome := job.Outcome{
		JobID:           "job1",
		ParentToolUseID: "parent1",
		Status:          job.StatusCancelled,
		Summary:         "  stopped halfway  ",
	}
	if !s.ApplyOutcome(outcome) {
		t.Fatal("first outcome must change the view")
	}
	if s.ApplyOutcome(outcome) {
		t.Fatal("the same outcome twice must not redraw")
	}

	run, ok := s.Run("parent1", "")
	if !ok {
		t.Fatal("no run")
	}
	if run.Summary != "stopped halfway" || run.Status != job.StatusCancelled {
		t.Fatalf("%+v", run)
	}
	if run.Running() {
		t.Fatal("an outcome ends the run")
	}
}

func TestSubagentStoreIgnoresUnfinishedOutcome(t *testing.T) {
	s := transcript.NewSubagentStore()
	if s.ApplyOutcome(job.Outcome{JobID: "job1", Status: job.StatusRunning}) {
		t.Fatal("a running job has no outcome to show")
	}
}
