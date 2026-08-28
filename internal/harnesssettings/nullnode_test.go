package harnesssettings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/plangate"
)

func TestManagerNullDefaultsNodeMeansNotConfigured(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("models:\n  - name: m\n    api_key: k\nplan:\n  defaults:\n"), 0o600))
	runtime, err := plangate.NewRuntime(plangate.DefaultDefaults())
	require.NoError(t, err)

	manager, err := harnesssettings.Open(path, runtime, nil)
	require.NoError(t, err)

	snapshot := manager.Snapshot()
	assert.Equal(t, plangate.DefaultDefaults(), snapshot.Plan,
		"a null plan.defaults node means not configured, matching LoadPlanDefaults")
}
