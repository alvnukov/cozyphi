package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnsupportedLegacyEffortDoesNotBecomeProviderModel(t *testing.T) {
	ctrl := newEffortController(t)
	require.NoError(t, ctrl.SetModelEffort("openai/gpt-5.5", "low"))
	before := ctrl.ModelSelectionStatus()
	require.ErrorContains(t, ctrl.SetModel("openai/gpt-5.5:ultra"), "does not support")
	require.Equal(t, before, ctrl.ModelSelectionStatus())
	require.ErrorContains(t, ctrl.SetModelEffort("openai/gpt-5.5:ultra", ""), "does not support")
	require.Equal(t, before, ctrl.ModelSelectionStatus())
}
