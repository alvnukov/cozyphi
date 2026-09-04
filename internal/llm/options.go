package llm

import "maps"

// ModelOptions is the request-tuning fragment a model entry (or one of its
// variants) carries onto a provider request. Every field is optional:
// pointers, empty strings and nil maps separate "unset" from a zero value so
// an overlay can leave a field alone. The JSON names are the openai wire
// names — the shared vocabulary for parsing opencode configs and for the
// chat-completions request body; other adapters map the fields they carry.
type ModelOptions struct {
	// Temperature and TopP are the sampling knobs both the openai and
	// anthropic wire formats carry.
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	// ReasoningEffort is a plain string, not the ReasoningEffort type in
	// types.go: configurations name provider-specific depths ("xhigh", "max")
	// that may not exist as enum values. Empty means unset.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	// ChatTemplateKwargs carries vLLM chat-template switches such as
	// {"thinking": true}.
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
	// EnableThinking is the Qwen-style reasoning switch; it stays a pointer
	// because an explicit false must reach the wire.
	EnableThinking *bool `json:"enable_thinking,omitempty"`
	// Thinking is the raw deepseek/zai reasoning request body
	// ({"type":"enabled","budget_tokens":…}); any JSON value round-trips.
	Thinking any `json:"thinking,omitempty"`
}

// VariantOptions is one named variant of a model: an options fragment that
// overlays the model's own options when the variant is selected. Disabled
// marks the variant as withdrawn — the opencode import drops such variants
// outright, and EffectiveOptions treats a survivor carrying the flag as if
// it were absent, so a disabled variant never contributes request fields.
type VariantOptions struct {
	Disabled bool         `json:"disabled,omitempty"`
	Options  ModelOptions `json:"options,omitempty"`
}

// MergeOptions overlays src over dst the way opencode merges model options:
// object fields (chat_template_kwargs, thinking) merge recursively, every
// other set value replaces its base, unset values pass the base through.
// Both inputs stay untouched; map results are fresh copies.
func MergeOptions(dst, src ModelOptions) ModelOptions {
	// merged starts as a value copy of dst; each set src field replaces it.
	merged := dst
	if src.Temperature != nil {
		merged.Temperature = src.Temperature
	}
	if src.TopP != nil {
		merged.TopP = src.TopP
	}
	if src.ReasoningEffort != "" {
		merged.ReasoningEffort = src.ReasoningEffort
	}
	if src.EnableThinking != nil {
		merged.EnableThinking = src.EnableThinking
	}
	if len(src.ChatTemplateKwargs) > 0 {
		merged.ChatTemplateKwargs = mergeValueMaps(dst.ChatTemplateKwargs, src.ChatTemplateKwargs)
	}
	if src.Thinking != nil {
		merged.Thinking = mergeValues(dst.Thinking, src.Thinking)
	}
	return merged
}

// EffectiveOptions returns the options a request for this model should
// carry: the model's own options with the selected variant's fragment
// overlaid, mirroring opencode's request-time merge where the chosen variant
// wins over everything else. A model without a selected variant — or one
// whose selection names no variant — sends its own options unchanged.
func (c ModelConfig) EffectiveOptions() ModelOptions {
	variant, ok := c.Variants[string(c.ReasoningEffort)]
	if !ok || variant.Disabled {
		return c.Options
	}
	return MergeOptions(c.Options, variant.Options)
}

// EffectiveReasoningEffort is the reasoning depth a request should send: the
// effective options' passthrough when the configuration named one, else the
// selected effort level. The passthrough wins because it is exactly what the
// selected variant (or the model's base options) asked the wire to carry.
func (c ModelConfig) EffectiveReasoningEffort() string {
	if effort := c.EffectiveOptions().ReasoningEffort; effort != "" {
		return effort
	}
	return string(c.ReasoningEffort)
}

// mergeValues merges two arbitrary option values: two objects merge
// recursively, anything else replaces dst with src.
func mergeValues(dst, src any) any {
	dstMap, dstOK := dst.(map[string]any)
	srcMap, srcOK := src.(map[string]any)
	if dstOK && srcOK {
		return mergeValueMaps(dstMap, srcMap)
	}
	return src
}

// mergeValueMaps deep-merges src into dst and returns a fresh map.
func mergeValueMaps(dst, src map[string]any) map[string]any {
	merged := make(map[string]any, len(dst)+len(src))
	maps.Copy(merged, dst)
	for key, value := range src {
		if base, ok := merged[key].(map[string]any); ok {
			if overlay, ok := value.(map[string]any); ok {
				merged[key] = mergeValueMaps(base, overlay)
				continue
			}
		}
		merged[key] = value
	}
	return merged
}
