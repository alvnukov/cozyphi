package agent

import (
	"fmt"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

// planStepModelName resolves the model one step runs on: the step's own
// override first, then the plan's per-type map. Empty means the session
// default.
func planStepModelName(plan session.Plan, stepID string) string {
	for _, item := range plan.Items {
		if item.ID != stepID {
			continue
		}
		if item.Model != "" {
			return item.Model
		}
		return plan.ModelsByType[item.Type]
	}
	return ""
}

// resolveStepModel turns a pinned model name into a usable config before
// anything else fires: a name the configuration cannot produce refuses the
// transition, so the plan never starts a step it cannot run.
func (engine *Engine) resolveStepModel(stepID, name string) (llm.ModelConfig, bool, error) {
	if name == "" {
		return llm.ModelConfig{}, false, nil
	}
	engine.mu.RLock()
	resolve := engine.resolveModel
	engine.mu.RUnlock()
	if resolve == nil {
		return llm.ModelConfig{}, false, fmt.Errorf(
			"step %q pins model %q, but the session has no model configuration to resolve it", stepID, name)
	}
	cfg, ok := resolve(name)
	if !ok {
		return llm.ModelConfig{}, false, fmt.Errorf(
			"step %q pins model %q, which is not configured; add the model or clear the pin", stepID, name)
	}
	return cfg, true, nil
}

// switchStepModel moves the engine onto the model a step pinned. The first
// swap inside a plan remembers the session model; a later step without a pin
// returns to it.
func (engine *Engine) switchStepModel(target llm.ModelConfig, pinned bool) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if !pinned {
		if engine.planModelActive {
			engine.setModelLocked(engine.planModelSaved)
			engine.planModelActive = false
		}
		return nil
	}
	if !engine.planModelActive {
		engine.planModelSaved = engine.modelCfg
		engine.planModelActive = true
	}
	engine.setModelLocked(target)
	return nil
}

// restoreSessionModelOnClose puts the session back on its own model when a
// plan closes, so step models never outlive the plan that pinned them.
func (engine *Engine) restoreSessionModelOnClose() {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.planModelActive {
		engine.setModelLocked(engine.planModelSaved)
		engine.planModelActive = false
	}
}
