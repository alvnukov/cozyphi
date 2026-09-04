package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMergeOptions pins opencode's options merge as one behavior table: a set
// source value replaces the base, an unset one passes the base through, and
// object-valued fields (chat_template_kwargs, thinking) merge recursively
// instead of replacing. The expected fragments are literals from opencode's
// documented config shape, never recomputed by MergeOptions itself.
func TestMergeOptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		base ModelOptions
		over ModelOptions
		want ModelOptions
	}{
		{
			name: "unset overlay passes the base through",
			base: ModelOptions{Temperature: new(1.0), ReasoningEffort: "high"},
			want: ModelOptions{Temperature: new(1.0), ReasoningEffort: "high"},
		},
		{
			name: "set overlay values replace the base",
			base: ModelOptions{
				Temperature: new(0.1), TopP: new(0.9), ReasoningEffort: "low",
				EnableThinking: new(true),
			},
			over: ModelOptions{
				Temperature: new(1.0), ReasoningEffort: "max", EnableThinking: new(false),
			},
			want: ModelOptions{
				Temperature: new(1.0), TopP: new(0.9), ReasoningEffort: "max",
				EnableThinking: new(false),
			},
		},
		{
			name: "chat template kwargs merge key by key",
			base: ModelOptions{ChatTemplateKwargs: map[string]any{"thinking": true, "keep": 1}},
			over: ModelOptions{ChatTemplateKwargs: map[string]any{"thinking": false}},
			want: ModelOptions{ChatTemplateKwargs: map[string]any{"thinking": false, "keep": 1}},
		},
		{
			name: "thinking objects merge, scalars replace",
			base: ModelOptions{Thinking: map[string]any{"type": "enabled", "budget_tokens": 8192}},
			over: ModelOptions{Thinking: map[string]any{"budget_tokens": 4096}},
			want: ModelOptions{Thinking: map[string]any{"type": "enabled", "budget_tokens": 4096}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, MergeOptions(test.base, test.over))
		})
	}
}

// TestMergeOptionsDoesNotAliasInputs: the merged options are a fresh value —
// adapters hand them to JSON encoding and the import stores the fragments, so
// a later mutation of one must never bleed into the other.
func TestMergeOptionsDoesNotAliasInputs(t *testing.T) {
	t.Parallel()
	base := ModelOptions{ChatTemplateKwargs: map[string]any{"thinking": true}}
	over := ModelOptions{ChatTemplateKwargs: map[string]any{"budget": 1}}
	merged := MergeOptions(base, over)
	merged.ChatTemplateKwargs["thinking"] = false
	assert.Equal(t, map[string]any{"thinking": true}, base.ChatTemplateKwargs)
}

// TestEffectiveOptionsAppliesSelectedVariant: selecting an effort that names a
// variant overlays that variant's fragment over the model's own options — the
// request-time merge order opencode uses, variant winning over everything.
func TestEffectiveOptionsAppliesSelectedVariant(t *testing.T) {
	t.Parallel()
	cfg := ModelConfig{
		Options: ModelOptions{
			Temperature: new(0.6), ReasoningEffort: "high",
			ChatTemplateKwargs: map[string]any{"thinking": true},
		},
		Variants: map[string]VariantOptions{
			"low": {
				Options: ModelOptions{ReasoningEffort: "low", ChatTemplateKwargs: map[string]any{"thinking": false}},
			},
		},
	}
	cfg.ReasoningEffort = ReasoningEffortLow
	assert.Equal(t, ModelOptions{
		Temperature: new(0.6), ReasoningEffort: "low",
		ChatTemplateKwargs: map[string]any{"thinking": false},
	}, cfg.EffectiveOptions())

	// An effort naming no variant sends the model's own options unchanged.
	cfg.ReasoningEffort = ReasoningEffortHigh
	assert.Equal(t, cfg.Options, cfg.EffectiveOptions())

	// A disabled variant never contributes, even when selected.
	cfg.Variants["high"] = VariantOptions{Disabled: true}
	assert.Equal(t, cfg.Options, cfg.EffectiveOptions())
}

// TestEffectiveReasoningEffortPrefersOptionsPassthrough: the options fragment
// is exactly what the configuration asked the wire to carry, so it wins over
// the selected effort; with no passthrough the selection itself is the value.
func TestEffectiveReasoningEffortPrefersOptionsPassthrough(t *testing.T) {
	t.Parallel()
	cfg := ModelConfig{
		ReasoningEffort: ReasoningEffortHigh,
		Variants: map[string]VariantOptions{
			"high": {Options: ModelOptions{ReasoningEffort: "max"}},
		},
	}
	assert.Equal(t, "max", cfg.EffectiveReasoningEffort())

	cfg.Variants = nil
	assert.Equal(t, "high", cfg.EffectiveReasoningEffort())

	cfg.ReasoningEffort = ""
	cfg.Options = ModelOptions{ReasoningEffort: "xhigh"}
	assert.Equal(t, "xhigh", cfg.EffectiveReasoningEffort())
}
