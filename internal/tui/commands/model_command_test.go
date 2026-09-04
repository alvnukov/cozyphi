package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModelCommandWithoutArgsOpensPicker: "/model" with no argument is
// the shared picker's keyboard entry — it must open the picker, not
// guess or no-op.
func TestModelCommandWithoutArgsOpensPicker(t *testing.T) {
	r := NewBuiltinRegistry()
	host := &fakeHost{modelNames: []string{"gpt"}}

	require.True(t, r.DispatchSlash("/model", CommandContext{Host: host}))
	assert.Equal(t, 1, host.openedModelPicker, "the empty /model must open the shared picker")
}

// TestModelCommandWithArgStillApplies: the text path keeps working — an
// argument applies the model directly, as before.
func TestModelCommandWithArgStillApplies(t *testing.T) {
	r := NewBuiltinRegistry()
	r.RegisterModelCommand([]string{"gpt", "last-model"})
	host := &fakeHost{modelNames: []string{"gpt", "last-model"}}

	require.True(t, r.DispatchSlash("/model gpt", CommandContext{Host: host}))
	assert.Equal(t, "gpt", host.model)
}

// TestEffortCommandRemoved: effort is chosen inside the model picker,
// not through a separate slash command anymore.
func TestEffortCommandRemoved(t *testing.T) {
	r := NewBuiltinRegistry()
	host := &fakeHost{effortLvls: []string{"low", "high"}}

	require.False(t, r.DispatchSlash("/effort high", CommandContext{Host: host}),
		"/effort must no longer dispatch")
	assert.Empty(t, host.effort, "no effort may be applied by the removed command")
}
