package session

import (
	"testing"
	"time"
)

func thinkingMessage(id string, dur time.Duration, state State) Message {
	return Message{
		ID: id, Role: RoleAssistant, State: state, ThinkingDuration: dur,
		Content: []ContentBlock{{Type: BlockThinking, Text: "segment " + id}},
	}
}

// TestProjectCopiesThinkingDuration: the engine-stamped reasoning span rides
// the projected thinking row so the header can say "Thought for 4s".
func TestProjectCopiesThinkingDuration(t *testing.T) {
	items := Project(Snapshot{Messages: []Message{thinkingMessage("a1", 4*time.Second, StateComplete)}})
	if len(items) != 1 || items[0].Kind != ItemThinking {
		t.Fatalf("items=%+v", items)
	}
	if items[0].ThinkingDuration != 4*time.Second {
		t.Fatalf("duration=%v, want 4s", items[0].ThinkingDuration)
	}
}

// TestProjectCoalesceSumsThinkingDuration: adjacent reasoning segments merge
// into one block; their spans add up so the merged header stays honest.
func TestProjectCoalesceSumsThinkingDuration(t *testing.T) {
	snap := Snapshot{Messages: []Message{
		thinkingMessage("a1", 3*time.Second, StateComplete),
		thinkingMessage("a2", 4*time.Second, StateComplete),
	}}
	items := Project(snap)
	if len(items) != 1 {
		t.Fatalf("items=%+v, want one coalesced thinking row", items)
	}
	if items[0].ThinkingDuration != 7*time.Second {
		t.Fatalf("duration=%v, want 7s", items[0].ThinkingDuration)
	}
}
