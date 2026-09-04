package agent

import (
	"fmt"
	"slices"

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

// resolveStepModel turns a pinned model reference into a usable config
// before anything else fires: the shared "name:effort" convention splits
// here, so the base name resolves and the effort rides onto the config the
// step runs on. A reference the configuration cannot produce refuses the
// transition, so the plan never starts a step it cannot run.
func (engine *Engine) resolveStepModel(stepID, ref string) (llm.ModelConfig, bool, error) {
	if ref == "" {
		return llm.ModelConfig{}, false, nil
	}
	name, effort := session.ParseModelRef(ref)
	engine.mu.RLock()
	resolve := engine.resolveModel
	engine.mu.RUnlock()
	if resolve == nil {
		return llm.ModelConfig{}, false, fmt.Errorf(
			"step %q pins model %q, but the session has no model configuration to resolve it", stepID, ref)
	}
	cfg, ok := resolve(name)
	if !ok {
		return llm.ModelConfig{}, false, fmt.Errorf(
			"step %q pins model %q, which is not configured; add the model or clear the pin", stepID, name)
	}
	if effort != "" {
		level := llm.ReasoningEffort(effort)
		if !slices.Contains(cfg.ReasoningEfforts, level) {
			return llm.ModelConfig{}, false, fmt.Errorf(
				"step %q pins model %q with effort %q, which it does not offer; pick one of its levels or clear the effort",
				stepID,
				name,
				effort,
			)
		}
		cfg.ReasoningEffort = level
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
