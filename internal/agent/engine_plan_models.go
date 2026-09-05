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

func planStepEffort(plan session.Plan, stepID string) string {
	for _, item := range plan.Items {
		if item.ID == stepID {
			return item.Effort
		}
	}
	return ""
}

// resolveStepModel resolves human-owned identity first, then overlays the
// independent effort. An effort-only step starts from the original session
// config, never the previous step's temporary override. Validate before effects.
func (engine *Engine) resolveStepModel(plan session.Plan, stepID string) (llm.ModelConfig, bool, error) {
	ref := planStepModelName(plan, stepID)
	override := planStepEffort(plan, stepID)
	if ref == "" && override == "" {
		return llm.ModelConfig{}, false, nil
	}
	name, effort := session.ParseModelRef(ref)
	engine.mu.RLock()
	resolve := engine.resolveModel
	cfg := engine.modelCfg
	if engine.planModelActive {
		cfg = engine.planModelSaved
	}
	engine.mu.RUnlock()
	if ref != "" {
		if resolve == nil {
			return llm.ModelConfig{}, false, fmt.Errorf(
				"step %q pins model %q, but the session has no model configuration to resolve it",
				stepID,
				ref,
			)
		}
		var ok bool
		cfg, ok = resolve(name)
		if !ok {
			return llm.ModelConfig{}, false, fmt.Errorf(
				"step %q pins model %q, which is not configured; add the model or clear the pin",
				stepID,
				name,
			)
		}
	}
	if override != "" {
		level, ok := llm.ParseReasoningEffort(override)
		if !ok || level == "" {
			return llm.ModelConfig{}, false, fmt.Errorf(
				"step %q has invalid effort %q; clear it or choose a supported level",
				stepID,
				override,
			)
		}
		effort = string(level)
	}
	name = cfg.Name
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
