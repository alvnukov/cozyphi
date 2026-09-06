package transcript_test

import (
	"strings"
	"testing"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

// spawnSnapshot is one parent turn that spawned a child: the assistant's
// agent_spawn call and the row the tool wrote back.
func spawnSnapshot(detail string) session.Snapshot {
	return session.Snapshot{
		Messages: []session.Message{{
			ID:    "m1",
			Role:  session.RoleAssistant,
			State: session.StateComplete,
			Content: []session.ContentBlock{{
				Type:     session.BlockToolUse,
				ID:       "call_agent",
				Name:     "agent_spawn",
				Input:    `{"role":"worker","description":"fix the lexer","prompt":"p"}`,
				Complete: true,
			}},
		}},
		Tools: map[string]session.ToolRun{
			"call_agent": {
				ToolUseID: "call_agent",
				Name:      "agent_spawn",
				Status:    session.ToolDone,
				Detail:    detail,
				Output:    `{"job_id":"job_1","status":"starting"}`,
			},
		},
	}
}

func syncAgentRow(t *testing.T, m *transcript.Mapper, snap session.Snapshot) *block.AgentBlock {
	t.Helper()
	entries, _, _ := m.Sync(nil, nil, snap)
	if len(entries) != 1 {
		t.Fatalf("entries=%d", len(entries))
	}
	ab, ok := entries[0].(*block.AgentBlock)
	if !ok {
		t.Fatalf("got %T", entries[0])
	}
	return ab
}

// TestMapperAgentRowIsNamedAfterTheChild: the row title is role(description),
// not the tool that made it — and a history written before that format keeps
// what it recorded rather than falling back to raw input JSON.
func TestMapperAgentRowIsNamedAfterTheChild(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	if got := syncAgentRow(t, m, spawnSnapshot("worker(fix the lexer)")).Name; got != "worker(fix the lexer)" {
		t.Fatalf("title %q", got)
	}

	// Old histories carry the pre-parentheses detail; it still reads as the
	// row's name.
	m = transcript.NewMapper(components.DefaultTheme(), nil, nil)
	if got := syncAgentRow(t, m, spawnSnapshot("worker: fix the lexer")).Name; got != "worker: fix the lexer" {
		t.Fatalf("legacy title %q", got)
	}

	// A row whose tool recorded no detail is named from the call's arguments,
	// never from the raw JSON the projection backfills.
	m = transcript.NewMapper(components.DefaultTheme(), nil, nil)
	ab := syncAgentRow(t, m, spawnSnapshot(""))
	if ab.Name != "worker(fix the lexer)" {
		t.Fatalf("empty detail title %q", ab.Name)
	}
	txt := components.SurfaceText(ab.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 20}}))
	if strings.Contains(txt, `"prompt"`) {
		t.Fatalf("raw input leaked into the title: %q", txt)
	}
}

// TestMapperAgentRowCountsToolsWhileRunning: a spawn call returns at once,
// but the row keeps the child's count and clock until an outcome lands.
func TestMapperAgentRowCountsToolsWhileRunning(t *testing.T) {
	store := transcript.NewSubagentStore()
	store.Bind("job_1", "call_agent")
	for _, id := range []string{"c1", "c2"} {
		store.ApplyProgress(job.Progress{
			JobID:           "job_1",
			ParentToolUseID: "call_agent",
			ToolUseID:       id,
			Name:            "read",
			Status:          "done",
			Detail:          id + ".go",
		})
	}

	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	m.Subagent = store.Run
	ab := syncAgentRow(t, m, spawnSnapshot("worker(fix the lexer)"))

	if ab.Tools != 2 || len(ab.Children) != 2 {
		t.Fatalf("tools=%d children=%d", ab.Tools, len(ab.Children))
	}
	if ab.Status != status.ToolRunning {
		t.Fatalf("a child still working must not read done: %v", ab.Status)
	}
	if ab.Started.IsZero() || !ab.Finished.IsZero() {
		t.Fatalf("running clock: started=%v finished=%v", ab.Started, ab.Finished)
	}
}

// TestMapperAgentRowSettlesOnOutcome: the delivered outcome gives the row its
// glyph, its summary and a stopped clock — in the same block, no extra row.
func TestMapperAgentRowSettlesOnOutcome(t *testing.T) {
	cases := []struct {
		name   string
		status job.Status
		want   status.ToolStatus
	}{
		{"done", job.StatusCompleted, status.ToolDone},
		{"error", job.StatusFailed, status.ToolError},
		{"stopped", job.StatusCancelled, status.ToolCancelled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := transcript.NewSubagentStore()
			store.Bind("job_1", "call_agent")
			store.ApplyProgress(job.Progress{
				JobID: "job_1", ParentToolUseID: "call_agent",
				ToolUseID: "c1", Name: "read", Status: "done", Detail: "a.go",
			})
			store.ApplyOutcome(job.Outcome{
				JobID:           "job_1",
				ParentToolUseID: "call_agent",
				Status:          tc.status,
				Summary:         "## Findings\n\n- the lexer eats commas",
			})

			m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
			m.Subagent = store.Run
			ab := syncAgentRow(t, m, spawnSnapshot("worker(fix the lexer)"))

			if ab.Status != tc.want {
				t.Fatalf("status %v want %v", ab.Status, tc.want)
			}
			if !strings.Contains(ab.Summary, "the lexer eats commas") {
				t.Fatalf("summary %q", ab.Summary)
			}
			if ab.Finished.IsZero() {
				t.Fatal("an outcome stops the clock")
			}
			if ab.Tools != 1 {
				t.Fatalf("tools=%d", ab.Tools)
			}
		})
	}
}
