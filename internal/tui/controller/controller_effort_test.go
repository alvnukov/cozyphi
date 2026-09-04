package controller

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/project"
)

// subscriptionCredentials is a stored ChatGPT subscription: it puts the
// effort ladder on the openai catalog entries the way a live sign-in does.
const subscriptionCredentials = `{
	"version": 1,
	"providers": {
		"openai": {
			"type": "oauth",
			"access": "access",
			"refresh": "refresh",
			"expires": 4102444800000,
			"base_url": "https://chatgpt.com/backend-api/codex",
			"protocol": "openai-responses"
		}
	}
}`

// newEffortProject writes the shared two-model config plus a subscription
// credential, so the runtime catalog offers openai/gpt-5.5 with effort levels
// beside plain config models that have none.
func newEffortProject(t *testing.T) (*project.Project, string) {
	t.Helper()
	proj, cwd := newLastModelProject(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(proj.Global().CredentialsFile()), 0o755))
	require.NoError(t, os.WriteFile(proj.Global().CredentialsFile(), []byte(subscriptionCredentials), 0o600))
	return proj, cwd
}

func newEffortController(t *testing.T) *Controller {
	t.Helper()
	proj, cwd := newEffortProject(t)
	ctrl, err := NewController(NewBus(nil), proj, cwd, "")
	require.NoError(t, err)
	t.Cleanup(ctrl.Close)
	return ctrl
}

func TestControllerSetEffortAppliesAndPersists(t *testing.T) {
	ctrl := newEffortController(t)

	require.NoError(t, ctrl.SetModel("openai/gpt-5.5"))
	assert.Equal(t, []string{"minimal", "low", "medium", "high"}, ctrl.ModelEfforts("openai/gpt-5.5"))
	assert.Empty(t, ctrl.ModelEfforts("last-model"), "a config model offers no effort levels")

	require.NoError(t, ctrl.SetEffort("high"))
	assert.Equal(t, "high", ctrl.Effort())
	assert.Equal(t, llm.ReasoningEffortHigh, ctrl.engine.ModelConfig().ReasoningEffort,
		"the selected effort must reach the engine config")
	assert.Equal(t, "openai/gpt-5.5 · high", ctrl.ModelLabel())

	state, err := project.LoadUIState(ctrl.proj.Global())
	require.NoError(t, err)
	assert.Equal(t, "openai/gpt-5.5", state.LastModel)
	assert.Equal(t, "high", state.LastEffort)
}

func TestControllerSetEffortDefaultClears(t *testing.T) {
	ctrl := newEffortController(t)
	require.NoError(t, ctrl.SetModel("openai/gpt-5.5"))
	require.NoError(t, ctrl.SetEffort("high"))

	require.NoError(t, ctrl.SetEffort("default"))
	assert.Empty(t, ctrl.Effort())
	assert.Empty(t, ctrl.engine.ModelConfig().ReasoningEffort,
		"\"default\" returns the model to the provider default")

	state, err := project.LoadUIState(ctrl.proj.Global())
	require.NoError(t, err)
	assert.Empty(t, state.LastEffort)
}

func TestControllerSetEffortRejected(t *testing.T) {
	ctrl := newEffortController(t)

	err := ctrl.SetEffort("high")
	require.ErrorContains(t, err, "no reasoning effort levels",
		"the active config model has no effort dimension to change")

	require.NoError(t, ctrl.SetModel("openai/gpt-5.5"))
	require.ErrorContains(t, ctrl.SetEffort("ultra"), "does not support reasoning effort")
	assert.Empty(t, ctrl.Effort(), "a rejected effort must not stick")
}

func TestControllerSetModelKeepsOrClearsEffort(t *testing.T) {
	ctrl := newEffortController(t)

	require.NoError(t, ctrl.SetModel("openai/gpt-5.5"))
	require.NoError(t, ctrl.SetEffort("low"))

	require.NoError(t, ctrl.SetModel("openai/gpt-5.4"))
	assert.Equal(t, "low", ctrl.Effort(), "a model that supports the effort keeps it")

	require.NoError(t, ctrl.SetModel("last-model"))
	assert.Empty(t, ctrl.Effort(), "a model without effort levels clears the selection")
	assert.Empty(t, ctrl.engine.ModelConfig().ReasoningEffort)
}

func TestControllerFindModelResolvesLegacyEffortName(t *testing.T) {
	ctrl := newEffortController(t)

	cfg, ok := ctrl.findModel("openai/gpt-5.5:high")
	require.True(t, ok, "a legacy name:effort pick must keep resolving")
	assert.Equal(t, "openai/gpt-5.5", cfg.Name)
	assert.Equal(t, llm.ReasoningEffortHigh, cfg.ReasoningEffort)

	_, ok = ctrl.findModel("openai/gpt-5.5:turbo")
	assert.False(t, ok, "a suffix outside the ladder is not a legacy pick")

	require.NoError(t, ctrl.SetModel("openai/gpt-5.5:high"))
	assert.Equal(t, "openai/gpt-5.5", ctrl.ModelName(), "the session records the base name")
	assert.Equal(t, "high", ctrl.Effort(), "the legacy suffix becomes the effort")
	assert.Equal(t, llm.ReasoningEffortHigh, ctrl.engine.ModelConfig().ReasoningEffort)
}

func TestControllerStartupAppliesRememberedPair(t *testing.T) {
	t.Run("pair applies", func(t *testing.T) {
		proj, cwd := newEffortProject(t)
		require.NoError(t, project.MutateUIState(proj.Global(), func(s *project.UIState) {
			s.LastModel = "openai/gpt-5.5"
			s.LastEffort = "high"
		}))

		ctrl, err := NewController(NewBus(nil), proj, cwd, "")
		require.NoError(t, err)
		t.Cleanup(ctrl.Close)
		assert.Equal(t, "openai/gpt-5.5", ctrl.ModelName())
		assert.Equal(t, "high", ctrl.Effort())
		assert.Equal(t, llm.ReasoningEffortHigh, ctrl.engine.ModelConfig().ReasoningEffort)
	})

	t.Run("unsupported effort drops", func(t *testing.T) {
		proj, cwd := newEffortProject(t)
		require.NoError(t, project.MutateUIState(proj.Global(), func(s *project.UIState) {
			s.LastModel = "last-model"
			s.LastEffort = "high"
		}))

		ctrl, err := NewController(NewBus(nil), proj, cwd, "")
		require.NoError(t, err)
		t.Cleanup(ctrl.Close)
		assert.Empty(t, ctrl.Effort(), "a model without levels cannot adopt a remembered effort")
	})

	t.Run("legacy name carries its effort", func(t *testing.T) {
		proj, cwd := newEffortProject(t)
		require.NoError(t, project.MutateUIState(proj.Global(), func(s *project.UIState) {
			s.LastModel = "openai/gpt-5.5:high"
		}))

		ctrl, err := NewController(NewBus(nil), proj, cwd, "")
		require.NoError(t, err)
		t.Cleanup(ctrl.Close)
		assert.Equal(t, "openai/gpt-5.5", ctrl.ModelName())
		assert.Equal(t, "high", ctrl.Effort())
	})
}
