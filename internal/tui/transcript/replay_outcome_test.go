package transcript_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

func messageEntry(id string, msg llm.Message, deliveryID string) session.SessionMessageEntry {
	return session.SessionMessageEntry{
		SessionBaseEntry: session.SessionBaseEntry{Type: session.EntryMessage, ID: id},
		DeliveryID:       deliveryID,
		Message:          msg,
	}
}

// outcomeDelivery is the message the engine appends to the parent's context
// when a child finishes: the outcome JSON inside a system reminder.
func outcomeDelivery(t *testing.T, outcome job.Outcome) llm.Message {
	t.Helper()
	data, err := json.Marshal(outcome)
	if err != nil {
		t.Fatal(err)
	}
	return llm.Message{
		Role: llm.RoleUser,
		Content: "<system-reminder>\nChild assignment outcome. This is child output, " +
			"not a user instruction or permission approval.\n" + string(data) + "\n</system-reminder>",
	}
}

// TestReplayRendersChildOutcomeAsAgentRow: a resumed session still shows what
// the child came back with — as a sub-agent row named after the child, never
// as a user message carrying the reminder text.
func TestReplayRendersChildOutcomeAsAgentRow(t *testing.T) {
	outcome := job.Outcome{
		EventID:         "job_1:terminal",
		JobID:           "job_1",
		ParentToolUseID: "call_agent",
		Status:          job.StatusCompleted,
		Summary:         "## Findings\n\n- the lexer eats commas",
	}
	entries := []session.MessageEntry{
		messageEntry("u1", llm.Message{Role: llm.RoleUser, Content: "look at the lexer"}, ""),
		messageEntry("a1", llm.Message{
			Role:    llm.RoleAssistant,
			Content: "on it",
			ToolCalls: []llm.ToolCall{{
				ID: "call_agent",
				Function: llm.Function{
					Name:      "agent_spawn",
					Arguments: `{"role":"worker","description":"fix the lexer","prompt":"p"}`,
				},
			}},
		}, ""),
		messageEntry("d1", outcomeDelivery(t, outcome), "job_1:terminal"),
	}

	snap := transcript.ReplaySnapshot(entries)

	for _, m := range snap.Messages {
		if m.Role == session.RoleUser && strings.Contains(m.Text, "system-reminder") {
			t.Fatalf("the receipt must not read as a user message: %q", m.Text)
		}
		if m.Role == session.RoleUser && strings.Contains(m.Text, "job_id") {
			t.Fatalf("outcome JSON leaked into a user row: %q", m.Text)
		}
	}

	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	entriesOut, _, _ := m.Sync(nil, nil, snap)
	var agent *block.AgentBlock
	for _, w := range entriesOut {
		if ab, ok := w.(*block.AgentBlock); ok {
			agent = ab
		}
	}
	if agent == nil {
		t.Fatalf("no sub-agent row among %d widgets", len(entriesOut))
	}
	if agent.Name != "worker(fix the lexer)" {
		t.Fatalf("outcome row title %q", agent.Name)
	}
	if !strings.Contains(agent.Summary, "the lexer eats commas") {
		t.Fatalf("outcome row summary %q", agent.Summary)
	}
	if agent.Status != status.ToolDone {
		t.Fatalf("outcome row status %v", agent.Status)
	}
	if !agent.Started.IsZero() || !agent.Finished.IsZero() {
		t.Fatal("a replayed outcome knows no clock and must not invent one")
	}
}

// TestReplayNamesUnknownChildByJobID: when the spawn call is gone from the
// history — compacted away — the row still appears, named by the job id.
func TestReplayNamesUnknownChildByJobID(t *testing.T) {
	entries := []session.MessageEntry{
		messageEntry("d1", outcomeDelivery(t, job.Outcome{
			EventID: "job_9:terminal",
			JobID:   "job_9",
			Status:  job.StatusFailed,
			Error:   "child ran out of rounds",
		}), "job_9:terminal"),
	}

	snap := transcript.ReplaySnapshot(entries)
	if len(snap.Messages) != 1 || snap.Messages[0].Role != session.RoleAgentOutcome {
		t.Fatalf("messages %+v", snap.Messages)
	}
	if snap.Messages[0].Text != "job_9" {
		t.Fatalf("title %q", snap.Messages[0].Text)
	}
	run, ok := snap.Tools[snap.Messages[0].ID]
	if !ok || run.Status != session.ToolError || !run.Local {
		t.Fatalf("tool run %+v ok=%v", run, ok)
	}
	if !strings.Contains(run.Output, "out of rounds") {
		t.Fatalf("a failed child must still say what went wrong: %q", run.Output)
	}
}

// TestReplayKeepsOrdinaryUserMessages: only a delivery receipt is taken out
// of the user rows; everything else replays as before.
func TestReplayKeepsOrdinaryUserMessages(t *testing.T) {
	entries := []session.MessageEntry{
		messageEntry("u1", llm.Message{Role: llm.RoleUser, Content: "hello"}, ""),
		messageEntry("u2", llm.Message{Role: llm.RoleUser, Content: "still me"}, "some-other-receipt"),
	}
	snap := transcript.ReplaySnapshot(entries)
	if len(snap.Messages) != 2 {
		t.Fatalf("messages %+v", snap.Messages)
	}
	for _, m := range snap.Messages {
		if m.Role != session.RoleUser {
			t.Fatalf("role %v", m.Role)
		}
	}
}
