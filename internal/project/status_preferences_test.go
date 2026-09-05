package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreferredStatusTab(t *testing.T) {
	for _, tc := range []struct {
		name   string
		counts map[string]uint64
		want   string
	}{
		{"default", nil, "usage"},
		{"most closed", map[string]uint64{"config": 4, "usage": 2, "stats": 3}, "config"},
		{"usage tie", map[string]uint64{"status": 4, "usage": 4}, "usage"},
		{"other tie", map[string]uint64{"status": 4, "config": 4}, "status"},
		{"unknown ignored", map[string]uint64{"other": 99}, "usage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := UIState{StatusCloses: tc.counts}
			assert.Equal(t, tc.want, state.PreferredStatusTab())
		})
	}
}

func TestStatusClosuresPersistWithoutChangingOtherPreferences(t *testing.T) {
	global := GlobalLayout{root: t.TempDir()}
	require.NoError(t, MutateUIState(global, func(s *UIState) {
		s.LastModel = "example"
		s.RecordStatusClose("stats")
		s.RecordStatusClose("stats")
		s.RecordStatusClose("usage")
		s.RecordStatusClose("unknown")
	}))
	state, err := LoadUIState(global)
	require.NoError(t, err)
	assert.Equal(t, "stats", state.PreferredStatusTab())
	assert.Equal(t, "example", state.LastModel)
	assert.Equal(t, map[string]uint64{"stats": 2, "usage": 1}, state.StatusCloses)
	// Opening/reading a preference does not record a closure.
	assert.Equal(t, "stats", state.PreferredStatusTab())
	assert.Equal(t, uint64(2), state.StatusCloses["stats"])
}

func TestStatusClosureCounterDoesNotWrap(t *testing.T) {
	state := UIState{StatusCloses: map[string]uint64{"usage": ^uint64(0)}}
	state.RecordStatusClose("usage")
	assert.Equal(t, ^uint64(0), state.StatusCloses["usage"])
}
