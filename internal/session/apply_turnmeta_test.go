package session

import (
	"testing"
	"time"
)

// TestApplyPreservesTurnTiming: a terminal update that omits model/start
// (e.g. a synthesized row) keeps what streaming events already established,
// while its own End timestamp wins — mirroring the Usage preservation rule.
func TestApplyPreservesTurnTiming(t *testing.T) {
	started := time.Now()
	s := Apply(Snapshot{}, AssistantMessageUpdate{Message: Message{
		ID: "a1", State: StateStreaming, Model: "m", Started: started,
		Content: []ContentBlock{{Type: BlockText, Text: "partial"}},
	}})
	ended := started.Add(5 * time.Second)
	s = Apply(s, AssistantMessageUpdate{Message: Message{
		ID: "a1", State: StateComplete, Ended: ended,
		Content: []ContentBlock{{Type: BlockText, Text: "done"}},
	}})
	got := s.Messages[0]
	if got.Model != "m" || !got.Started.Equal(started) || !got.Ended.Equal(ended) {
		t.Fatalf("timing not preserved: model=%q started=%v ended=%v", got.Model, got.Started, got.Ended)
	}
}
