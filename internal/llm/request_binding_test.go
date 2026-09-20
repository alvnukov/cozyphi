package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestBindingFingerprintTracksEffectiveRequest(t *testing.T) {
	temperature, topP, variantTemperature := 0.2, 0.9, 0.4
	base := ModelConfig{
		Name:               "reader",
		APIName:            "gpt-reader",
		ProviderID:         "openai",
		Protocol:           ProtocolOpenAI,
		APIKey:             "credential-one",
		BaseURL:            "https://gateway.example/v1",
		ConnectionIdentity: "account-fingerprint",
		ContextWindow:      128_000,
		MaxOutputTokens:    4_096,
		ReasoningEffort:    ReasoningEffortHigh,
		Options:            ModelOptions{Temperature: &temperature, TopP: &topP},
		Variants: map[string]VariantOptions{
			"high": {Options: ModelOptions{Temperature: &variantTemperature}},
		},
	}
	fingerprint := base.RequestBindingFingerprint()
	require.NotEmpty(t, fingerprint)

	credentialRotation := base
	credentialRotation.APIKey = "credential-two"
	assert.Equal(t, fingerprint, credentialRotation.RequestBindingFingerprint(),
		"credential rotation does not replace the request recipient or shape")

	nonRequestMetadata := base
	nonRequestMetadata.ContextWindow = 256_000
	nonRequestMetadata.SkillPath = "/different/skills"
	nonRequestMetadata.ReasoningEfforts = []ReasoningEffort{ReasoningEffortLow, ReasoningEffortHigh}
	assert.Equal(t, fingerprint, nonRequestMetadata.RequestBindingFingerprint(),
		"catalog capabilities and local paths are not request configuration")

	changes := map[string]func(*ModelConfig){
		"configured model": func(cfg *ModelConfig) { cfg.Name = "other-reader" },
		"request model":    func(cfg *ModelConfig) { cfg.APIName = "gpt-other" },
		"provider":         func(cfg *ModelConfig) { cfg.ProviderID = "other-provider" },
		"protocol":         func(cfg *ModelConfig) { cfg.Protocol = ProtocolAnthropic },
		"endpoint":         func(cfg *ModelConfig) { cfg.BaseURL = "https://other.example/v1" },
		"account":          func(cfg *ModelConfig) { cfg.ConnectionIdentity = "other-account" },
		"output limit":     func(cfg *ModelConfig) { cfg.MaxOutputTokens++ },
		"effort":           func(cfg *ModelConfig) { cfg.ReasoningEffort = ReasoningEffortLow },
		"effective option": func(cfg *ModelConfig) {
			changedTopP := 0.8
			cfg.Options.TopP = &changedTopP
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			changed := base
			change(&changed)
			assert.NotEqual(t, fingerprint, changed.RequestBindingFingerprint())
		})
	}
}
