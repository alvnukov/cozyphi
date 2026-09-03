package harnesssettings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/tasks"
)

// The General tab's task row: a missing permissions.tasks reads as write,
// a click steps the level, and a save lands under permissions without
// disturbing the neighbors the user wrote by hand.
func TestManagerTaskAccessDefaultsCyclesAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(
		t,
		os.WriteFile(path, []byte("permissions:\n  mode: interactive\nopencode:\n  future: keep\n"), 0o600),
	)
	manager, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.NoError(t, err)
	assert.Equal(t, tasks.AccessWrite, manager.Snapshot().Tasks)

	draft := manager.Snapshot().Draft()
	assert.Equal(t, tasks.AccessWrite, draft.Tasks)
	draft.CycleTasksAccess()
	assert.Equal(t, tasks.AccessAsk, draft.Tasks)

	applied, err := manager.Apply(t.Context(), draft)
	require.NoError(t, err)
	assert.Equal(t, tasks.AccessAsk, applied.Tasks)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "permissions:\n  mode: interactive\n  tasks: ask\n",
		"the level joins the permissions section the user already has")
	assert.Contains(t, string(data), "opencode:\n  future: keep\n", "unrelated sections survive")

	reopened, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.NoError(t, err)
	assert.Equal(t, tasks.AccessAsk, reopened.Snapshot().Tasks)
}

// "off" is a YAML 1.1 boolean; the level must survive a round trip as the
// word, and a value that is none of the four is a config error naming them,
// the same one the session would raise.
func TestManagerTaskAccessRoundTripsOffAndRejectsUnknown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("permissions:\n  tasks: off\n"), 0o600))
	manager, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.NoError(t, err)
	assert.Equal(t, tasks.AccessOff, manager.Snapshot().Tasks)

	draft := manager.Snapshot().Draft()
	draft.CycleTasksAccess()
	assert.Equal(t, tasks.AccessWrite, draft.Tasks, "off wraps back to write")
	draft.Tasks = tasks.AccessOff
	_, err = manager.Apply(t.Context(), draft)
	require.NoError(t, err)
	reopened, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.NoError(t, err)
	assert.Equal(t, tasks.AccessOff, reopened.Snapshot().Tasks)

	require.NoError(t, os.WriteFile(path, []byte("permissions:\n  tasks: maybe\n"), 0o600))
	_, err = harnesssettings.Open(path, mustRuntime(t), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `permissions.tasks: unknown value "maybe"`)
}
