package controller

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/session"
)

// TestActivityStreamingFooterSplit: while streaming, the footer stays live —
// its spinner ticks and its label names the model producing the round, so
// the screen never goes quiet before the first block or between blocks. The
// transcript's "model · thinking" wave stays the feed's own indicator.
func TestActivityStreamingFooterSplit(t *testing.T) {
	h := NewActivityHandler(status.NewSpinner(components.DefaultTheme().ToolName))
	h.Apply(ActivityStreaming)
	if !h.ShowSpinner() {
		t.Fatal("common spinner must keep ticking while streaming")
	}
	if !h.ShowFooterSpinner() {
		t.Fatal("footer spinner must keep ticking while streaming")
	}
	streaming := session.Snapshot{Messages: []session.Message{
		{Role: session.RoleAssistant, State: session.StateStreaming, Model: "deepseek-v4-pro"},
	}}
	if got := h.FooterLabel(streaming); got != "deepseek-v4-pro" {
		t.Fatalf("footer label = %q, want the live model", got)
	}
	if got := h.FooterLabel(session.Snapshot{}); got != "Generating…" {
		t.Fatalf("pre-token footer label = %q, want the Generating… fallback", got)
	}
	if h.Label(session.Snapshot{}) == "" {
		t.Fatal("sidebar label keeps the streaming state")
	}

	h.Apply(ActivityTools)
	if !h.ShowFooterSpinner() || h.FooterLabel(session.Snapshot{}) == "" {
		t.Fatal("non-streaming activities keep their footer status")
	}
}
