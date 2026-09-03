package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/project"
)

const testLastModelConfig = `models:
  - name: default-model
    api_key: k
    base_url: %[1]s
    default: true
  - name: last-model
    api_key: k
    base_url: %[1]s
`

// newLastModelProject writes a two-model config. baseURL points the models at
// a test server for the cases that need a real turn; the default is a dead
// port, which is enough for startup-only assertions.
func newLastModelProject(t *testing.T, baseURL ...string) (*project.Project, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	url := "http://127.0.0.1:9"
	if len(baseURL) > 0 {
		url = baseURL[0]
	}
	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		proj.Global().ConfigFile(), fmt.Appendf(nil, testLastModelConfig, url), 0o644,
	))
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

// Resuming adopts the session's recorded model, which startup deliberately
// ignores. Recording it would move the next fresh session's model behind the
// user's back.
func TestControllerResumeDoesNotOverwriteLastModel(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	proj, cwd := newLastModelProject(t, server.URL)
	ctrl, err := NewController(NewBus(nil), proj, cwd, "")
	require.NoError(t, err)
	t.Cleanup(ctrl.Close)

	// A session is only resumable once it holds a turn.
	ctrl.StartPrompt("seed a resumable session", nil, "seed")
	waitForCond(t, 10*time.Second, func() bool { return requests.Load() >= 1 && !ctrl.RunActive() })
	resumable := ctrl.SessionID()
	require.NotEmpty(t, resumable)

	require.NoError(t, ctrl.SetModel("last-model"))
	_, err = ctrl.Resume(resumable)
	require.NoError(t, err)
	require.Equal(t, "default-model", ctrl.ModelName(), "the resumed session runs its own model")

	state, err := project.LoadUIState(proj.Global())
	require.NoError(t, err)
	assert.Equal(t, "last-model", state.LastModel, "the user's own pick survives a resume")
}
