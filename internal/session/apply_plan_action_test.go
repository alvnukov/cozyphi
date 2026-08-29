package session

import (
	"strings"
	"testing"
)

// TestPlanActionRanAppendsTranscriptRow pins the UI-only row one executed plan
// action leaves behind: a local plan tool row the transcript renders with the
// widget it already has, stamped by the run's terminal status.
func TestPlanActionRanAppendsTranscriptRow(t *testing.T) {
	s := Apply(Snapshot{}, PlanActionRan{
		StepID: "work", Event: PlanActionOnStepStart, Type: PlanActionCompact, Status: PlanActionRunOK,
	})

	if len(s.Messages) != 1 || s.Messages[0].Role != RolePlan {
		t.Fatalf("messages: %+v", s.Messages)
	}
	run := s.Tools[s.Messages[0].ID]
	if run.ToolUseID != s.Messages[0].ID || !run.Local || run.Status != ToolDone {
		t.Fatalf("tool run: %+v", run)
	}
	if !strings.Contains(run.Detail, "compact") || !strings.Contains(run.Detail, "step_start") {
		t.Fatalf("detail must name the action and its event: %q", run.Detail)
	}
}

// TestPlanActionRanFailedRowCarriesError keeps the failure legible: a failed
// run closes its row as failed and carries the bounded error text.
func TestPlanActionRanFailedRowCarriesError(t *testing.T) {
	s := Apply(Snapshot{}, PlanActionRan{
		Event: PlanActionOnPlanEnd, Type: PlanActionCompact, Status: PlanActionRunFailed, Error: "compaction declined",
	})

	run := s.Tools[s.Messages[0].ID]
	if run.Status != ToolError {
		t.Fatalf("status: %+v", run)
	}
	if !strings.Contains(run.Output, "compaction declined") {
		t.Fatalf("error text must ride the row: %+v", run)
	}
}

// TestProjectRendersPlanActionRow projects the row as a local tool entry, so
// the transcript needs no new widget to show it.
func TestProjectRendersPlanActionRow(t *testing.T) {
	s := Apply(Snapshot{}, PlanActionRan{
		Event: PlanActionOnStepEnd, Type: PlanActionInjectSkill, Status: PlanActionRunOK,
	})

	items := Project(s)
	found := false
	for _, item := range items {
		if item.Kind == ItemTool && item.ToolName == "⚙ plan" {
			found = true
			if item.ToolRun.Status != ToolDone || !item.ToolRun.Local {
				t.Fatalf("projected run: %+v", item.ToolRun)
			}
		}
	}
	if !found {
		t.Fatalf("no plan action row projected: %+v", items)
	}
}
