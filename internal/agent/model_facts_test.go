package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/llm"
)

// sentinelAuthenticator is a credential holder that would be caught by name
// if the projection ever walked the whole config instead of listing fields.
type sentinelAuthenticator struct{ token string }

func (sentinelAuthenticator) Authorize(context.Context, *http.Request) error { return nil }

// secretModel is a model configuration whose every credential-bearing field
// holds a distinct sentinel, so a leak names which field leaked.
func secretModel() llm.ModelConfig {
	return llm.ModelConfig{
		Name:            "public-name",
		APIName:         "public-api-name",
		ProviderID:      "public-provider",
		Protocol:        llm.ProtocolOpenAI,
		APIKey:          "sk-test-secret",
		BaseURL:         "https://secret-endpoint.invalid/v1",
		Authenticator:   sentinelAuthenticator{token: "authenticator-secret"},
		SkillPath:       "/home/someone/.cozyphi/skills",
		ContextWindow:   128000,
		MaxOutputTokens: 4096,
	}
}

func TestModelFactsCarriesNoCredential(t *testing.T) {
	encoded, err := json.Marshal(ModelFacts(secretModel()))
	require.NoError(t, err)

	for _, secret := range []string{
		"sk-test-secret",
		"secret-endpoint.invalid",
		"authenticator-secret",
		"/home/someone",
	} {
		assert.NotContains(t, string(encoded), secret,
			"the projection lists the fields it exports; nothing else can ride along")
	}
	assert.Contains(t, string(encoded), "public-name")
	assert.Contains(t, string(encoded), "public-provider",
		"which provider a model belongs to is a catalog fact, not a credential")
}

func TestModelFactsReportsACredentialAsPresenceAndKindOnly(t *testing.T) {
	cases := []struct {
		name string
		cfg  llm.ModelConfig
		has  bool
		kind string
	}{{
		name: "a stored key",
		cfg:  llm.ModelConfig{Name: "m", APIKey: "sk-test-secret"},
		has:  true,
		kind: "api_key",
	}, {
		name: "a request authenticator",
		cfg:  llm.ModelConfig{Name: "m", Authenticator: sentinelAuthenticator{token: "authenticator-secret"}},
		has:  true,
		kind: "authenticator",
	}, {
		// The authenticator wins because it is the one that signs the request.
		name: "both, where the token is what actually signs",
		cfg: llm.ModelConfig{
			Name: "m", APIKey: "sk-test-secret",
			Authenticator: sentinelAuthenticator{token: "authenticator-secret"},
		},
		has:  true,
		kind: "authenticator",
	}, {
		name: "neither",
		cfg:  llm.ModelConfig{Name: "m"},
	}}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			facts := ModelFacts(item.cfg)

			assert.Equal(t, item.has, facts.Credential)
			assert.Equal(t, item.kind, facts.CredentialKind)

			encoded, err := json.Marshal(facts)
			require.NoError(t, err)
			for _, secret := range []string{"sk-test-secret", "authenticator-secret", "sk-test", "secret"} {
				assert.NotContains(t, string(encoded), secret,
					"a credential is reported as presence and kind: never a value, a hash or a suffix")
			}
		})
	}
}

func TestModelFactsNamesRequestOptionsWithoutTheirValues(t *testing.T) {
	cfg := secretModel()
	cfg.Options = llm.ModelOptions{
		Temperature:        new(0.123456),
		ChatTemplateKwargs: map[string]any{"kwarg-key-sentinel": "kwarg-value-sentinel"},
		Thinking:           map[string]any{"budget_tokens": "thinking-body-sentinel"},
		EnableThinking:     new(true),
	}

	facts := ModelFacts(cfg)
	encoded, err := json.Marshal(facts)
	require.NoError(t, err)

	assert.Equal(t,
		[]string{"temperature", "chat_template_kwargs", "enable_thinking", "thinking"},
		facts.Options,
		"which knobs a request carries is safe to say; what they are set to is not")
	for _, value := range []string{"0.123456", "kwarg-key-sentinel", "kwarg-value-sentinel", "thinking-body-sentinel"} {
		assert.NotContains(t, string(encoded), value,
			"an option value is whatever a config file or an import put there")
	}
}

func TestModelFactsProjectsWhatARequestWouldUse(t *testing.T) {
	cfg := secretModel()
	cfg.ReasoningEffort = llm.ReasoningEffortHigh
	cfg.ReasoningEfforts = []llm.ReasoningEffort{
		llm.ReasoningEffortMax,
		llm.ReasoningEffortLow,
		llm.ReasoningEffortHigh,
	}
	cfg.Options = llm.ModelOptions{Temperature: new(0.2)}
	cfg.Variants = map[string]llm.VariantOptions{
		"high":      {Options: llm.ModelOptions{ReasoningEffort: "high"}},
		"low":       {Options: llm.ModelOptions{TopP: new(0.9)}},
		"withdrawn": {Disabled: true},
		"max":       {Options: llm.ModelOptions{ReasoningEffort: "max"}},
	}

	facts := ModelFacts(cfg)

	assert.True(t, facts.Known)
	assert.Equal(t, "public-name", facts.Name)
	assert.Equal(t, "public-api-name", facts.RequestName, "the wire name is what the provider is asked for")
	assert.Equal(t, "openai", facts.Protocol)
	assert.Equal(t, "high", facts.Effort)
	assert.Equal(t, "high", facts.RequestEffort)
	assert.Equal(t, []string{"low", "high", "max"}, facts.EffortLevels, "depth order, not map order")
	assert.Equal(t, 128000, facts.ContextWindow)
	assert.Equal(t, 4096, facts.MaxOutputTokens)
	assert.Equal(t, []string{"high", "low", "max"}, facts.Variants, "a withdrawn variant is not a capability")
	assert.Equal(t, []string{"temperature", "reasoning_effort"}, facts.Options,
		"the selected variant's overlay is part of what a request carries")
	assert.False(t, facts.Thinking)
}

func TestModelFactsFallsBackToTheOnlyNameItHas(t *testing.T) {
	facts := ModelFacts(llm.ModelConfig{Name: "deepseek-reasoner"})

	assert.Equal(t, "deepseek-reasoner", facts.RequestName, "a config with no wire name sends the selector")
	assert.True(t, facts.Thinking)
	assert.Empty(t, facts.EffortLevels, "a model with no runtime levels claims none")
	assert.Empty(t, facts.Variants)
	assert.Empty(t, facts.Options)
}

func TestModelFactsDoesNotAliasTheConfigItRead(t *testing.T) {
	cfg := llm.ModelConfig{
		Name:             "m",
		ReasoningEfforts: []llm.ReasoningEffort{llm.ReasoningEffortLow, llm.ReasoningEffortHigh},
	}
	original := append([]llm.ReasoningEffort(nil), cfg.ReasoningEfforts...)

	facts := ModelFacts(cfg)
	facts.EffortLevels[0] = "tampered"

	assert.Equal(t, original, cfg.ReasoningEfforts, "sorting a copy leaves the owner's order alone")
	assert.Equal(t, []string{"low", "high"}, ModelFacts(cfg).EffortLevels)
}

// observedEngine is an engine with a model that offers two depths, which is
// the smallest thing an effort choice can be tested against.
func observedEngine(t *testing.T) *Engine {
	t.Helper()
	engine, err := NewEngine(EngineOpts{
		Model: llm.ModelConfig{
			Name:             "first",
			APIKey:           "sk-test-secret",
			BaseURL:          "http://127.0.0.1:9",
			ContextWindow:    100000,
			ReasoningEfforts: []llm.ReasoningEffort{llm.ReasoningEffortLow, llm.ReasoningEffortHigh},
		},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)
	return engine
}

func TestModelObservationFollowsTheEngineAndItsGeneration(t *testing.T) {
	engine := observedEngine(t)

	before := engine.ModelObservation()
	require.True(t, before.Known)
	assert.Equal(t, "first", before.Loaded.Name)
	assert.Equal(t, "first", before.Acting.Name, "an idle engine acts on what it holds")
	assert.Equal(t, 100000, before.Window)
	assert.False(t, before.Pinned)
	assert.Equal(t, "0", before.Revision, "a fresh engine has swapped models zero times")

	require.NoError(t, engine.SelectModel(llm.ModelConfig{
		Name:             "second",
		ContextWindow:    50000,
		ReasoningEfforts: []llm.ReasoningEffort{llm.ReasoningEffortLow, llm.ReasoningEffortHigh},
	}, llm.ReasoningEffortHigh))

	after := engine.ModelObservation()
	assert.Equal(t, "second", after.Loaded.Name)
	assert.Equal(t, "high", after.Loaded.Effort)
	assert.Equal(t, 50000, after.Window)
	assert.NotEqual(t, before.Revision, after.Revision,
		"two observations of the same model generation must be distinguishable from two generations")
}

func TestARejectedEffortLeavesTheObservationUntouched(t *testing.T) {
	engine := observedEngine(t)
	cfg := llm.ModelConfig{
		Name:             "second",
		ReasoningEfforts: []llm.ReasoningEffort{llm.ReasoningEffortLow, llm.ReasoningEffortHigh},
	}

	require.Error(t, engine.SelectModel(cfg, llm.ReasoningEffortMax),
		"a depth the model does not offer is not a depth")

	state := engine.ModelObservation()
	assert.Equal(t, "first", state.Loaded.Name, "a refused switch is not a switch")
	assert.Empty(t, state.Loaded.Effort, "no effort was ever set, and none is invented")
	assert.Equal(t, "0", state.Revision)
}

func TestTheObservedWindowIsTheOneTheEngineBudgetsAgainst(t *testing.T) {
	engine := observedEngine(t)

	engine.SetContextWindowOverride(20000)

	state := engine.ModelObservation()
	assert.Equal(t, 100000, state.Loaded.ContextWindow, "the model's own window is unchanged")
	assert.Equal(t, 20000, state.Window)
	assert.Equal(t, engine.ContextWindow(), state.Window)
}

func TestAStepPinIsObservableAndOutlivesNothing(t *testing.T) {
	engine := observedEngine(t)
	step := llm.ModelConfig{Name: "step-model", ContextWindow: 40000}

	require.NoError(t, engine.switchStepModel(step, true))

	pinned := engine.ModelObservation()
	assert.True(t, pinned.Pinned)
	assert.Equal(t, "step-model", pinned.Loaded.Name)

	engine.restoreSessionModelOnClose()

	restored := engine.ModelObservation()
	assert.False(t, restored.Pinned, "the plan closed; nothing pins the model any more")
	assert.Equal(t, "first", restored.Loaded.Name)
	assert.Equal(t, 100000, restored.Window)
	assert.NotEqual(t, pinned.Revision, restored.Revision)
}

func TestNoEngineObservesNothingRatherThanGuessing(t *testing.T) {
	var engine *Engine

	state := engine.ModelObservation()

	assert.Equal(t, diag.ModelState{}, state)
	assert.False(t, state.Known)
}
