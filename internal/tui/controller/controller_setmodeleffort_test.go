package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/project"
)

// TestControllerSetModelEffortAppliesAndPersists: the picker commits a
// model and its effort as one choice — both reach the engine config and
// the remembered pair in one step.
func TestControllerSetModelEffortAppliesAndPersists(t *testing.T) {
	ctrl := newEffortController(t)

	require.NoError(t, ctrl.SetModelEffort("openai/gpt-5.5", "high"))
	assert.Equal(t, "openai/gpt-5.5", ctrl.ModelName())
	assert.Equal(t, "high", ctrl.Effort())
	assert.Equal(t, llm.ReasoningEffortHigh, ctrl.engine.ModelConfig().ReasoningEffort,
		"the committed effort must reach the engine config")
	assert.Equal(t, "openai/gpt-5.5 · high", ctrl.ModelLabel())

	state, err := project.LoadUIState(ctrl.proj.Global())
	require.NoError(t, err)
	assert.Equal(t, "openai/gpt-5.5", state.LastModel)
	assert.Equal(t, "high", state.LastEffort)
}

// TestControllerSetModelEffortDefaultClears: "default" is the picker's
// clear token — the model applies, the effort returns to the provider
// depth, and nothing effort-shaped is remembered.
func TestControllerSetModelEffortDefaultClears(t *testing.T) {
	ctrl := newEffortController(t)
	require.NoError(t, ctrl.SetModelEffort("openai/gpt-5.5", "high"))

	require.NoError(t, ctrl.SetModelEffort("openai/gpt-5.5", "default"))
	assert.Empty(t, ctrl.Effort())
	assert.Empty(t, ctrl.engine.ModelConfig().ReasoningEffort)
	assert.Equal(t, "openai/gpt-5.5", ctrl.ModelLabel())

	state, err := project.LoadUIState(ctrl.proj.Global())
	require.NoError(t, err)
	assert.Empty(t, state.LastEffort)
}

// TestControllerSetModelEffortRejectedLeavesStateIntact: an effort the
// model does not offer fails closed — no half-applied model switch, and
// the previous selection survives untouched.
func TestControllerSetModelEffortRejectedLeavesStateIntact(t *testing.T) {
	ctrl := newEffortController(t)
	require.NoError(t, ctrl.SetModelEffort("openai/gpt-5.5", "low"))

	require.ErrorContains(t, ctrl.SetModelEffort("openai/gpt-5.5", "ultra"),
		"does not support reasoning effort")
	assert.Equal(t, "openai/gpt-5.5", ctrl.ModelName(), "the model pick must survive a rejected effort")
	assert.Equal(t, "low", ctrl.Effort(), "the previous effort must survive a rejected commit")

	require.ErrorContains(t, ctrl.SetModelEffort("last-model", "high"),
		"no reasoning effort levels")
	assert.Equal(t, "openai/gpt-5.5", ctrl.ModelName(),
		"a level-less model must not half-apply")

	state, err := project.LoadUIState(ctrl.proj.Global())
	require.NoError(t, err)
	assert.Equal(t, "openai/gpt-5.5", state.LastModel)
	assert.Equal(t, "low", state.LastEffort)
}

// TestControllerSetModelEffortWithoutEffortStep: committing a level-less
// model with no effort at all is the one-step path — allowed and clean.
func TestControllerSetModelEffortWithoutEffortStep(t *testing.T) {
	ctrl := newEffortController(t)

	require.NoError(t, ctrl.SetModelEffort("last-model", ""))
	assert.Equal(t, "last-model", ctrl.ModelName())
	assert.Empty(t, ctrl.Effort())
	assert.Equal(t, "last-model", ctrl.ModelLabel())
}
