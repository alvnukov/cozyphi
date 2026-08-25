package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestControllerDropContextEntriesGuardedWhileStreaming: deletions are
// refused while a reply or queued prompt runs, exactly like trims.
func TestControllerDropContextEntriesGuardedWhileStreaming(t *testing.T) {
	ctrl := newReadyController(t)
	ctrl.streamRunning = true

	err := ctrl.DropContextEntries([]string{"some-entry"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot delete")
}
