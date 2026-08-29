package controller

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/session"
)

// TestActivityLabels: the activity handler feeds the footer a phase label;
// the footer itself prefixes the working model next to it. Streaming keeps
// the shared ticker running — the transcript's "model · thinking" wave
// animates off it.
func TestActivityLabels(t *testing.T) {
	h := NewActivityHandler(status.NewSpinner(components.DefaultTheme().ToolName))

	h.Apply(ActivityStreaming)
	if !h.ShowSpinner() {
		t.Fatal("the shared ticker must keep running while streaming")
	}
	if got := h.Label(session.Snapshot{}); got != "Generating…" {
		t.Fatalf("streaming label = %q, want Generating…", got)
	}

	h.Apply(ActivityTools)
	twoTools := session.Snapshot{Tools: map[string]session.ToolRun{
		"a": {Name: "read", Status: session.ToolInProgress},
		"b": {Name: "bash", Status: session.ToolQueued},
	}}
	if got := h.Label(twoTools); got != "Calling 2 tools…" {
		t.Fatalf("tools label = %q, want Calling 2 tools…", got)
	}

	h.Apply(ActivityWaiting)
	if got := h.Label(session.Snapshot{}); got != "Awaiting reply…" {
		t.Fatalf("waiting label = %q, want Awaiting reply…", got)
	}
}
