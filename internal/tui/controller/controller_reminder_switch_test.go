package controller

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/project"
)

// TestClearKeepsConfiguredReminderThreshold pins the General reminder
// threshold across a session switch: /new builds a fresh engine, and the
// compact-advice ladder inside it must still budget against the configured
// value instead of silently falling back to the window-derived default.
func TestClearKeepsConfiguredReminderThreshold(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(proj.Global().ConfigFile(), fmt.Appendf(nil,
		`models:
  - name: test-model
    api_key: k
    base_url: http://127.0.0.1:9
    context_window: 163840
    default: true
`), 0o644))
	require.NoError(t, proj.LoadConfig())

	bus := NewBus(nil)
	ctrl, err := NewController(bus, proj, cwd, "")
	require.NoError(t, err)

	// The General value arrives the way View.applySettings delivers it
	// after harness settings load.
	ctrl.SetReminderThreshold(150_000)

	require.NoError(t, ctrl.Clear())

	compaction := ctrl.engine.ContextObservation().Compaction
	require.Equal(t, 150_000, compaction.ReminderTokens,
		"the fresh engine carries the configured reminder value")
	require.Equal(t, 150_000, compaction.Threshold,
		"the ladder budgets against the configured threshold, not the window-derived default")
}
