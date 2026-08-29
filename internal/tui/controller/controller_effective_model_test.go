package controller

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// TestControllerEffectiveModelFollowsEngine: the sidebar status must name the
// engine's live model; before the engine exists it falls back to the session
// default rather than going blank.
func TestControllerEffectiveModelFollowsEngine(t *testing.T) {
	c := &Controller{modelCfg: llm.ModelConfig{Name: "session-default"}}
	if got := c.EffectiveModelName(); got != "session-default" {
		t.Fatalf("fallback = %q, want session-default", got)
	}

	ctrl := newReadyController(t)
	if ctrl.engine == nil {
		t.Fatal("ready controller has no engine")
	}
	if got := ctrl.EffectiveModelName(); got == "" || got != ctrl.ModelName() {
		t.Fatalf("engine model = %q, session model = %q", got, ctrl.ModelName())
	}
}
