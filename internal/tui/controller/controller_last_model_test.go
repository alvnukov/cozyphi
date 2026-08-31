package controller

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/project"
)

const testLastModelConfig = `models:
  - name: default-model
    api_key: k
    base_url: http://127.0.0.1:9
    default: true
  - name: last-model
    api_key: k
    base_url: http://127.0.0.1:9
`

func newLastModelProject(t *testing.T) (*project.Project, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(proj.Global().ConfigFile(), []byte(testLastModelConfig), 0o644))
	require.NoError(t, proj.LoadConfig())
	return proj, cwd
}

func setLastModel(t *testing.T, proj *project.Project, name string) {
	t.Helper()
	require.NoError(t, project.MutateUIState(proj.Global(), func(s *project.UIState) {
		s.LastModel = name
	}))
}

func TestControllerStartupUsesLastModel(t *testing.T) {
	proj, cwd := newLastModelProject(t)
	setLastModel(t, proj, "last-model")

	ctrl, err := NewController(NewBus(nil), proj, cwd, "")
	require.NoError(t, err)
	assert.Equal(t, "last-model", ctrl.ModelName())
}

func TestControllerStartupFallsBackWhenLastModelStale(t *testing.T) {
	proj, cwd := newLastModelProject(t)
	setLastModel(t, proj, "missing-model")

	ctrl, err := NewController(NewBus(nil), proj, cwd, "")
	require.NoError(t, err)
	assert.Equal(t, "default-model", ctrl.ModelName())
}

func TestControllerStartupEnvOverridesLastModel(t *testing.T) {
	proj, cwd := newLastModelProject(t)
	setLastModel(t, proj, "last-model")
	t.Setenv("COZYPHI_MODEL", "env-model")

	ctrl, err := NewController(NewBus(nil), proj, cwd, "")
	require.NoError(t, err)
	assert.Equal(t, "env-model", ctrl.ModelName())
}

func TestControllerSetModelPersistsLastModel(t *testing.T) {
	proj, cwd := newLastModelProject(t)

	ctrl, err := NewController(NewBus(nil), proj, cwd, "")
	require.NoError(t, err)
	require.Equal(t, "default-model", ctrl.ModelName())

	require.NoError(t, ctrl.SetModel("last-model"))
	assert.Equal(t, "last-model", ctrl.ModelName())

	state, err := project.LoadUIState(proj.Global())
	require.NoError(t, err)
	assert.Equal(t, "last-model", state.LastModel)
}
