package controller

import (
	"fmt"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/llm"
)

// ModelSelectionStatus exposes the user's choice separately from plan precedence
// and the immutable inference/tool round currently running. Read it on existing
// stream/status bus updates and immediately after a successful picker commit.
type ModelSelectionStatus struct {
	Selected agent.ModelSelection
	agent.ModelStatus
}

func (c *Controller) ModelSelectionStatus() ModelSelectionStatus {
	if c == nil {
		return ModelSelectionStatus{}
	}
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	cfg := c.runtimeModel()
	selected := agent.ModelSelection{Name: cfg.Name, Effort: cfg.ReasoningEffort}
	status := agent.ModelStatus{Next: selected, Effective: selected}
	if c.engine != nil {
		status = c.engine.ModelStatus()
	}
	return ModelSelectionStatus{Selected: selected, ModelStatus: status}
}

func (c *Controller) resolveModelSelection(name string) (llm.ModelConfig, error) {
	if cfg, ok := c.findModel(name); ok {
		return cfg, nil
	}
	// A rejected legacy effort must not become an arbitrary provider model ID.
	if base, suffix, ok := splitLegacyEffortName(name); ok {
		if _, known := c.findModel(base); known {
			return llm.ModelConfig{}, fmt.Errorf("model %q does not support reasoning effort %q; choose an offered effort", base, suffix)
		}
	}
	// Preserve the existing arbitrary-model workflow for unlisted endpoints.
	cfg := c.proj.Config().Model()
	cfg.Name = name
	return cfg, nil
}
