package agent

import (
	"fmt"
	"slices"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// ModelSelection is a credential-free model/effort pair.
type ModelSelection struct {
	Name   string
	Effort llm.ReasoningEffort
}

// ModelStatus distinguishes the next configured request from the active round.
// When idle Effective equals Next. Plan pins may make Next differ from the
// controller's user selection.
type ModelStatus struct {
	Next      ModelSelection
	Effective ModelSelection
	Pending   bool
}

// SelectModel validates a user-selected effort before committing the complete
// model configuration. Empty effort keeps the provider-configured default.
// Tools, permission policy, callbacks and child ceilings are retained; the
// existing round snapshot keeps an in-flight request and its tools unchanged.
func (engine *Engine) SelectModel(cfg llm.ModelConfig, effort llm.ReasoningEffort) error {
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("empty model name")
	}
	if effort != "" {
		parsed, valid := llm.ParseReasoningEffort(string(effort))
		if !valid || !slices.Contains(cfg.ReasoningEfforts, parsed) {
			return fmt.Errorf("model %q does not support reasoning effort %q", cfg.Name, effort)
		}
		cfg.ReasoningEffort = parsed
	}
	return engine.SetModel(cfg)
}

// ModelStatus returns a coherent snapshot suitable for status rendering.
func (engine *Engine) ModelStatus() ModelStatus {
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	next := ModelSelection{Name: engine.modelCfg.Name, Effort: engine.modelCfg.ReasoningEffort}
	effective := next
	if engine.activeModel != nil {
		effective = *engine.activeModel
	}
	return ModelStatus{Next: next, Effective: effective, Pending: next != effective}
}

func (engine *Engine) finishModelRound() {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.activeModel = nil
}

// A late response must not restore calibration cleared by a model switch.
func (engine *Engine) noteRoundTokenObservation(rt roundRuntime, estimate int, usage llm.Usage) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.client == rt.client && usage.PromptTokens > 0 {
		engine.tokenObs = &tokenObservation{estimate: estimate, prompt: usage.PromptTokens}
	}
}
