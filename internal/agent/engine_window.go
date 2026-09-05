package agent

// effectiveWindow resolves the context window one engine works with: the
// session override, clamped to what the model actually offers. An override of
// 0 means "the model's own window"; a model without a known window trusts the
// override as the only number there is.
func effectiveWindow(modelWindow, override int) int {
	switch {
	case override <= 0:
		return modelWindow
	case modelWindow <= 0:
		return override
	case override < modelWindow:
		return override
	default:
		return modelWindow
	}
}

// SetContextWindowOverride narrows or restores the context window this engine
// budgets against — the compaction ladder, microcompaction triggers and the
// over-window refusal all follow it. The override lives in the session only:
// it is never persisted, and a model switch keeps it applied to the new
// model's window.
func (engine *Engine) SetContextWindowOverride(tokens int) {
	if engine == nil {
		return
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.contextOverride = max(tokens, 0)
	engine.contextWindow = effectiveWindow(engine.modelCfg.ContextWindow, engine.contextOverride)
}

// ContextWindow reports the window the engine budgets against right now —
// the effective value after the session override, not the model's raw one.
func (engine *Engine) ContextWindow() int {
	if engine == nil {
		return 0
	}
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	return engine.contextWindow
}
