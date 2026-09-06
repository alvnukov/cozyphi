package agent

import (
	"slices"
	"strconv"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/llm"
)

// ModelFacts projects one model configuration into the diagnostics DTO.
//
// It is the allowlist for everything the harness may say about a model, and
// it lives here rather than in internal/diag on purpose: llm.ModelConfig
// carries the api key, the base URL and the request authenticator, and
// keeping that struct out of the diagnostics package means no edit made
// there can ever reach a credential. Every member below is copied by name,
// so a field added to ModelConfig later is invisible here until someone
// decides it is safe and adds it deliberately.
func ModelFacts(cfg llm.ModelConfig) diag.ModelFacts {
	return diag.ModelFacts{
		Known:           true,
		Name:            cfg.Name,
		RequestName:     cfg.RequestModel(),
		Protocol:        string(cfg.Protocol),
		Effort:          string(cfg.ReasoningEffort),
		RequestEffort:   cfg.EffectiveReasoningEffort(),
		EffortLevels:    modelEffortLevels(cfg.ReasoningEfforts),
		ContextWindow:   cfg.ContextWindow,
		MaxOutputTokens: cfg.MaxOutputTokens,
		Variants:        modelVariantNames(cfg.Variants),
		Options:         modelOptionNames(cfg.EffectiveOptions()),
		Thinking:        llm.IsThinkingModel(cfg.RequestModel()),
		Provider:        cfg.ProviderID,
		Credential:      cfg.APIKey != "" || cfg.Authenticator != nil,
		CredentialKind:  modelCredentialKind(cfg),
	}
}

// modelCredentialKind names how a model entry authenticates, and only how. A
// stored key and a request authenticator are two different exposures — a key
// sits in a config file or the credential store and rides every request as
// it is, a token is minted for one request and expires — and telling them
// apart is the whole of what the harness may say. The authenticator wins
// where both are present, because it is the one that signs the request.
// Neither the key nor the token, nor any part or hash of either, is
// reachable from the answer.
func modelCredentialKind(cfg llm.ModelConfig) string {
	switch {
	case cfg.Authenticator != nil:
		return "authenticator"
	case cfg.APIKey != "":
		return "api_key"
	default:
		return ""
	}
}

// ModelObservation reads this engine's model state for the diagnostics
// registry: what it holds for the next round, what the round in flight is
// running on, the window it budgets against, whether a plan step pinned the
// model, and the generation all of that belongs to. One read lock covers the
// whole read, so the layers of a single observation cannot disagree with
// each other, and the projection runs after the lock is released rather than
// sorting and allocating under it.
//
// A nil engine is the "no engine yet" case, not an error: the answer says
// unknown and every layer fed from it reports unavailable.
func (engine *Engine) ModelObservation() diag.ModelState {
	if engine == nil {
		return diag.ModelState{}
	}
	engine.mu.RLock()
	cfg := engine.modelCfg
	acting := ModelSelection{Name: cfg.Name, Effort: cfg.ReasoningEffort}
	if engine.activeModel != nil {
		acting = *engine.activeModel
	}
	window := engine.contextWindow
	pinned := engine.planModelActive
	revision := engine.modelRev
	engine.mu.RUnlock()
	return diag.ModelState{
		Known:    true,
		Loaded:   ModelFacts(cfg),
		Acting:   diag.ModelActing{Name: acting.Name, Effort: string(acting.Effort)},
		Window:   window,
		Pinned:   pinned,
		Revision: strconv.FormatUint(revision, 10),
	}
}

// modelEffortLevels copies the levels a model offers at runtime into the
// ladder's own order, so two observations of the same model are comparable
// whatever order the catalog happened to list them in.
func modelEffortLevels(efforts []llm.ReasoningEffort) []string {
	if len(efforts) == 0 {
		return nil
	}
	sorted := slices.Clone(efforts)
	llm.SortReasoningEfforts(sorted)
	levels := make([]string, 0, len(sorted))
	for _, effort := range sorted {
		levels = append(levels, string(effort))
	}
	return levels
}

// modelVariantNames lists the variants a model offers, sorted. A disabled
// variant is left out: it contributes nothing to a request, so naming it
// would describe a capability the model does not have.
func modelVariantNames(variants map[string]llm.VariantOptions) []string {
	if len(variants) == 0 {
		return nil
	}
	names := make([]string, 0, len(variants))
	for name, variant := range variants {
		if variant.Disabled {
			continue
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// modelOptionNames names the request-tuning options that are set, and only
// their names. The vocabulary is closed and written out here because an
// option's value is whatever a config file or an imported provider put
// there — free-form JSON in the case of thinking — so no value from this
// struct is ever exported.
func modelOptionNames(options llm.ModelOptions) []string {
	var names []string
	if options.Temperature != nil {
		names = append(names, "temperature")
	}
	if options.TopP != nil {
		names = append(names, "top_p")
	}
	if options.ReasoningEffort != "" {
		names = append(names, "reasoning_effort")
	}
	if len(options.ChatTemplateKwargs) > 0 {
		names = append(names, "chat_template_kwargs")
	}
	if options.EnableThinking != nil {
		names = append(names, "enable_thinking")
	}
	if options.Thinking != nil {
		names = append(names, "thinking")
	}
	return names
}
