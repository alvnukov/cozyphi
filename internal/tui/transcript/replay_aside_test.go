package transcript_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

func chainEntry(id, parent string, at time.Time, msg llm.Message) session.SessionMessageEntry {
	entry := session.SessionMessageEntry{
		SessionBaseEntry: session.SessionBaseEntry{Type: session.EntryMessage, ID: id, Timestamp: at},
		Message:          msg,
	}
	if parent != "" {
		entry.ParentID = &parent
	}
	return entry
}

func placedAside(id, after, question string) session.PlacedAside {
	return session.PlacedAside{
		AsideEntry: session.AsideEntry{
			SessionBaseEntry: session.SessionBaseEntry{Type: session.EntryAside, ID: id},
			Question:         question,
			Answer:           "answer to " + question,
			Model:            "m",
		},
		After: after,
	}
}

// A replayed side question stands after the entry it follows, a finished row
// under the id it streamed under, and a tool result the feed does not draw
// still holds its place.
func TestReplayPutsAnAsideAfterItsEntry(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	entries := []session.MessageEntry{
		chainEntry("u1", "", t0, llm.Message{Role: llm.RoleUser, Content: "first"}),
		chainEntry("c1", "u1", t0.Add(time.Second), llm.Message{
			Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call", Function: llm.Function{Name: "read"}}},
		}),
		chainEntry(
			"r1",
			"c1",
			t0.Add(2*time.Second),
			llm.Message{Role: llm.RoleTool, Content: "file", ToolCallID: "call"},
		),
		chainEntry("a1", "r1", t0.Add(3*time.Second), llm.Message{Role: llm.RoleAssistant, Content: "done"}),
		chainEntry("u2", "a1", t0.Add(4*time.Second), llm.Message{Role: llm.RoleUser, Content: "second"}),
		chainEntry("a2", "u2", t0.Add(5*time.Second), llm.Message{Role: llm.RoleAssistant, Content: "again"}),
	}
	about := placedAside("x2", "a1", "about the first turn?")
	about.AnchorPreview = "prompt first"
	snap := transcript.ReplaySnapshot(entries,
		placedAside("x1", "r1", "mid-turn?"),
		about,
		placedAside("x3", "a2", "at the end?"),
	)

	assert.Equal(t, []string{"u1", "c1", "x1", "a1", "x2", "u2", "a2", "x3"}, rowIDs(snap))
	x2 := snap.Messages[4]
	assert.Equal(t, session.RoleAside, x2.Role)
	assert.Equal(t, session.StateComplete, x2.State)
	assert.Equal(t, "m", x2.Model)
	assert.Equal(t, session.AsideRow{
		Question: "about the first turn?", AnchorPreview: "prompt first", Answer: "answer to about the first turn?",
	}, x2.Aside)

	assert.Equal(t, []string{"u1", "c1", "a1", "u2", "a2"}, rowIDs(transcript.ReplaySnapshot(entries)),
		"without asides the replay is the conversation alone")
}

// A question asked right after a compaction follows its marker; one about
// history compacted away opens the feed; one whose entry the path lacks is
// kept at the end rather than dropped.
func TestReplayPlacesAsidesAroundACompaction(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	compaction := session.CompactionEntry{
		SessionBaseEntry: session.SessionBaseEntry{
			Type:      session.EntryCompaction,
			ID:        "cmp",
			Timestamp: t0.Add(2 * time.Second),
		},
		Compaction: session.Compaction{Summary: "summary", FirstKeptEntryID: "u1"},
	}
	entries := []session.MessageEntry{
		compaction,
		chainEntry("u1", "", t0, llm.Message{Role: llm.RoleUser, Content: "kept"}),
		chainEntry("a1", "u1", t0.Add(time.Second), llm.Message{Role: llm.RoleAssistant, Content: "kept answer"}),
		chainEntry("u2", "cmp", t0.Add(3*time.Second), llm.Message{Role: llm.RoleUser, Content: "after"}),
	}
	snap := transcript.ReplaySnapshot(entries,
		placedAside("old", "", "before it all?"),
		placedAside("gone", "missing", "lost?"),
		placedAside("fresh", "cmp", "after compaction?"),
	)
	assert.Equal(t, []string{"old", "u1", "a1", "cmp", "fresh", "u2", "gone"}, rowIDs(snap))
}
