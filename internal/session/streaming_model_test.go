package session

import "testing"

// TestStreamingModel: the snapshot names the model of the round that is
// streaming right now; complete rounds and empty snapshots say nothing.
func TestStreamingModel(t *testing.T) {
	streaming := Snapshot{Messages: []Message{
		{Role: RoleUser},
		{Role: RoleAssistant, State: StateStreaming, Model: "deepseek-v4-pro"},
	}}
	if got := StreamingModel(streaming); got != "deepseek-v4-pro" {
		t.Fatalf("StreamingModel = %q, want deepseek-v4-pro", got)
	}

	done := Snapshot{Messages: []Message{
		{Role: RoleAssistant, State: StateComplete, Model: "deepseek-v4-pro"},
	}}
	if got := StreamingModel(done); got != "" {
		t.Fatalf("complete round reports %q, want empty", got)
	}

	if got := StreamingModel(Snapshot{}); got != "" {
		t.Fatalf("empty snapshot reports %q, want empty", got)
	}
}
