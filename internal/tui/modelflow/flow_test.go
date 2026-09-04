package modelflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ladder = func(string) []string {
	return []string{"minimal", "low", "medium", "high"}
}

var noEfforts = func(string) []string { return nil }

// TestFlowModelWithoutEffortsFinishes: a model with no levels completes
// the pick in one step — no effort page ever opens for it.
func TestFlowModelWithoutEffortsFinishes(t *testing.T) {
	f := New()
	require.False(t, f.SelectModel("last-model", noEfforts("last-model")))
	assert.Equal(t, "last-model", f.Model())
	assert.Empty(t, f.Efforts(), "a level-less model exposes no effort choices")
}

// TestFlowModelWithEffortsOpensStep: the effort page lists "default"
// first, then the model's own levels in the order the catalog gives them.
func TestFlowModelWithEffortsOpensStep(t *testing.T) {
	f := New()
	require.True(t, f.SelectModel("openai/gpt-5.5", ladder("openai/gpt-5.5")))
	assert.Equal(t, []string{"default", "minimal", "low", "medium", "high"}, f.Efforts())
}

// TestFlowCommitEffort: committing a level returns the (model, effort)
// pair the caller must apply.
func TestFlowCommitEffort(t *testing.T) {
	f := New()
	require.True(t, f.SelectModel("openai/gpt-5.5", ladder("openai/gpt-5.5")))

	model, effort, ok := f.SelectEffort("high")
	require.True(t, ok)
	assert.Equal(t, "openai/gpt-5.5", model)
	assert.Equal(t, "high", effort)
}

// TestFlowCommitDefaultClears: "default" is a valid choice and means the
// provider-configured depth — an empty effort on the wire.
func TestFlowCommitDefaultClears(t *testing.T) {
	f := New()
	require.True(t, f.SelectModel("openai/gpt-5.5", ladder("openai/gpt-5.5")))

	model, effort, ok := f.SelectEffort("default")
	require.True(t, ok)
	assert.Equal(t, "openai/gpt-5.5", model)
	assert.Empty(t, effort)
}

// TestFlowRejectsUnknownEffort: a level outside the model's own list
// fails closed and keeps the pending model, so Esc back to the model
// page is the only way out of a wrong pick.
func TestFlowRejectsUnknownEffort(t *testing.T) {
	f := New()
	require.True(t, f.SelectModel("openai/gpt-5.5", ladder("openai/gpt-5.5")))

	_, _, ok := f.SelectEffort("ultra")
	require.False(t, ok)
	assert.Equal(t, "openai/gpt-5.5", f.Model(), "a rejected effort keeps the pending model")
	assert.NotEmpty(t, f.Efforts(), "the effort page stays open")
}

// TestFlowBackReturnsToModels: Back drops the pending model pick — the
// Esc-from-effort semantics every picker shares.
func TestFlowBackReturnsToModels(t *testing.T) {
	f := New()
	require.True(t, f.SelectModel("openai/gpt-5.5", ladder("openai/gpt-5.5")))

	f.Back()
	assert.Empty(t, f.Model())
	assert.Empty(t, f.Efforts())
}
