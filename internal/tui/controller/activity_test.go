package controller

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/session"
)

// TestActivityStreamingFooterSplit: while streaming, the common spinner keeps
// ticking (transcript widgets animate off it) but the footer stops painting
// its own status — the transcript row "model · thinking" is the indicator.
func TestActivityStreamingFooterSplit(t *testing.T) {
	h := NewActivityHandler(status.NewSpinner(components.DefaultTheme().ToolName))
	h.Apply(ActivityStreaming)
	if !h.ShowSpinner() {
		t.Fatal("common spinner must keep ticking while streaming")
	}
	if h.ShowFooterSpinner() {
		t.Fatal("footer spinner must rest while streaming")
	}
	if h.FooterLabel(session.Snapshot{}) != "" {
		t.Fatal("footer label must be empty while streaming")
	}
	if h.Label(session.Snapshot{}) == "" {
		t.Fatal("sidebar label keeps the streaming state")
	}

	h.Apply(ActivityTools)
	if !h.ShowFooterSpinner() || h.FooterLabel(session.Snapshot{}) == "" {
		t.Fatal("non-streaming activities keep their footer status")
	}
}
