package tui_test

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/block"
	"github.com/pulseaiclub/phi/internal/components/status"
	"github.com/pulseaiclub/phi/internal/job"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tui"
)

func TestMapperAgentBlockSummaryAndChildren(t *testing.T) {
	store := tui.NewSubagentStore()
	store.Bind("job_1", "call_agent")
	store.ApplyProgress(job.Progress{
		JobID:           "job_1",
		ParentToolUseID: "call_agent",
		ToolUseID:       "c1",
		Name:            "read",
		Status:          "done",
		Detail:          "x.go",
	})

	m := tui.NewMapper(components.DefaultTheme(), nil, nil)
	m.Children = store.Children
	m.ChildrenByJob = store.ChildrenByJob

	snap := session.Snapshot{
		Messages: []session.Message{{
			ID:    "m1",
			Role:  session.RoleAssistant,
			State: session.StateComplete,
			Content: []session.ContentBlock{{
				Type:     session.BlockToolUse,
				ID:       "call_agent",
				Name:     "agent_task",
				Input:    `{"prompt":"p"}`,
				Complete: true,
			}},
		}},
		Tools: map[string]session.ToolRun{
			"call_agent": {
				ToolUseID: "call_agent",
				Name:      "agent_task",
				Status:    session.ToolDone,
				Detail:    "completed",
				Output: `{
  "job_id": "job_1",
  "status": "completed",
  "summary": "## Findings\n\n- ok"
}`,
			},
		},
	}

	entries, ids, dirty := m.Sync(nil, nil, snap)
	if len(entries) != 1 || len(ids) != 1 {
		t.Fatalf("entries=%d ids=%d", len(entries), len(ids))
	}
	if len(dirty) != 1 || dirty[0] != 0 {
		t.Fatalf("dirty=%v want [0]", dirty)
	}
	ab, ok := entries[0].(*block.AgentBlock)
	if !ok {
		t.Fatalf("got %T", entries[0])
	}
	if ab.Summary == "" || !strings.Contains(ab.Summary, "Findings") {
		t.Fatalf("summary %q", ab.Summary)
	}
	if len(ab.Children) != 1 || ab.Children[0].Name != "read" {
		t.Fatalf("children %+v", ab.Children)
	}
	if ab.Status != status.ToolDone {
		t.Fatalf("status %v", ab.Status)
	}
	surf := ab.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 40}})
	txt := components.SurfaceText(surf)
	if strings.Contains(txt, `"job_id"`) {
		t.Fatalf("raw json leaked: %q", txt)
	}
}

func TestMapperAgentWaitSummaryOnly(t *testing.T) {
	store := tui.NewSubagentStore()
	store.Bind("job_w", "call_spawn")
	store.ApplyProgress(job.Progress{
		JobID:           "job_w",
		ParentToolUseID: "call_spawn",
		ToolUseID:       "c1",
		Name:            "grep",
		Status:          "done",
		Detail:          "Tree",
	})

	m := tui.NewMapper(components.DefaultTheme(), nil, nil)
	m.Children = store.Children
	m.ChildrenByJob = store.ChildrenByJob

	// In-progress wait: title only, no duplicated child tree.
	snap := session.Snapshot{
		Messages: []session.Message{{
			ID:    "m1",
			Role:  session.RoleAssistant,
			State: session.StateComplete,
			Content: []session.ContentBlock{{
				Type:     session.BlockToolUse,
				ID:       "call_wait",
				Name:     "agent_wait",
				Input:    `{"job_id":"job_w"}`,
				Complete: true,
			}},
		}},
		Tools: map[string]session.ToolRun{
			"call_wait": {
				ToolUseID: "call_wait",
				Name:      "agent_wait",
				Status:    session.ToolInProgress,
				Detail:    "job_w",
			},
		},
	}

	entries, _, _ := m.Sync(nil, nil, snap)
	ab, ok := entries[0].(*block.AgentBlock)
	if !ok {
		t.Fatalf("got %T", entries[0])
	}
	if len(ab.Children) != 0 {
		t.Fatalf("wait must not show spawn children: %+v", ab.Children)
	}

	// Done wait: markdown summary only.
	snap.Tools["call_wait"] = session.ToolRun{
		ToolUseID: "call_wait",
		Name:      "agent_wait",
		Status:    session.ToolDone,
		Detail:    "completed",
		Output: `{
  "job_id": "job_w",
  "status": "completed",
  "summary": "## Done\n\n- ok"
}`,
	}
	entries, _, dirty := m.Sync(entries, []string{"call_wait"}, snap)
	if len(dirty) != 1 || dirty[0] != 0 {
		t.Fatalf("dirty=%v want [0] after summary change", dirty)
	}
	ab, ok = entries[0].(*block.AgentBlock)
	if !ok {
		t.Fatalf("got %T", entries[0])
	}
	if len(ab.Children) != 0 {
		t.Fatalf("wait children still set: %+v", ab.Children)
	}
	if !strings.Contains(ab.Summary, "Done") {
		t.Fatalf("summary %q", ab.Summary)
	}
	if !ab.Expanded {
		t.Fatal("expected expand when summary present")
	}
}

func TestBusCoalesceJobProgress(t *testing.T) {
	b := tui.NewBus(nil)
	b.Publish(tui.JobProgressMsg{Progress: job.Progress{JobID: "j", ToolUseID: "t1", Status: "in-progress"}})
	b.Publish(tui.JobProgressMsg{Progress: job.Progress{JobID: "j", ToolUseID: "t1", Status: "done"}})
	b.Publish(tui.JobProgressMsg{Progress: job.Progress{JobID: "j", ToolUseID: "t2", Status: "done"}})
	batch := b.Drain()
	if len(batch) != 2 {
		t.Fatalf("len=%d want 2 (coalesced t1)", len(batch))
	}
	jp := batch[0].(tui.JobProgressMsg)
	if jp.Progress.Status != "done" {
		t.Fatalf("coalesced status %q", jp.Progress.Status)
	}
}
