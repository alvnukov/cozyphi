package controller

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/session"
)

// The startup-without-model contract: an empty config and no connected
// providers must still produce a working controller, the runtime catalog
// supplies a fallback when it can, and a submit without any model is refused
// with guidance instead of a doomed turn.

// newNoModelProject discovers a project whose global layout holds only the
// freshly planted commented template, with every model source (env, opencode
// install, provider credentials) isolated away from the test machine.
func newNoModelProject(t *testing.T) (*project.Project, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "")
	t.Setenv("COZYPHI_API_KEY", "")
	t.Setenv("COZYPHI_BASE_URL", "")
	// opencode's config lives outside HOME; keep the test machine's install
	// out of the catalog.
	t.Setenv("OPENCODE_CONFIG", "")
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))

	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	return proj, cwd
}

// connectOpenAI writes an API-key credential for the builtin OpenAI provider,
// so the connected-provider store has a live catalog without any network.
func connectOpenAI(t *testing.T, proj *project.Project) {
	t.Helper()
	creds := map[string]any{
		"openai": map[string]any{
			"type":     "api",
			"key":      "test-key",
			"base_url": "https://api.openai.com/v1",
			"protocol": "openai",
		},
	}
	data, err := json.Marshal(map[string]any{"version": 1, "providers": creds})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(proj.Global().CredentialsFile(), data, 0o600))
}

func TestNewControllerSucceedsWithoutAnyModel(t *testing.T) {
	proj, cwd := newNoModelProject(t)

	ctrl, err := NewController(NewBus(nil), proj, cwd, "")
	require.NoError(t, err)
	t.Cleanup(ctrl.Close)

	// The engine is live (session, tools, gate all real); only the LLM side
	// is empty, and that is waited on lazily by the refused submit below.
	require.NotNil(t, ctrl.engine)
	assert.Empty(t, ctrl.ModelName(), "no model is selected")
	assert.Empty(t, ctrl.ModelConfig().Name)

	// The footer/sidebar source shows a placeholder, not an empty string.
	assert.Equal(t, noModelLabel, ctrl.EffectiveModelName())

	notice := ctrl.ModelSetupNotice()
	require.NotEmpty(t, notice)
	assert.Contains(t, notice, "/connect")
	assert.Contains(t, notice, "/model")
	assert.Contains(t, notice, "config.yaml")
}

func TestNewControllerPicksFirstCatalogModel(t *testing.T) {
	proj, cwd := newNoModelProject(t)
	connectOpenAI(t, proj)

	ctrl, err := NewController(NewBus(nil), proj, cwd, "")
	require.NoError(t, err)
	t.Cleanup(ctrl.Close)

	catalog := ctrl.providers.Models()
	require.NotEmpty(t, catalog, "the connected provider store must supply models")
	assert.Equal(t, catalog[0].Name, ctrl.ModelName(), "startup picks the first catalog model")
	assert.NotEmpty(t, ctrl.ModelConfig().APIKey, "the picked model is connection-ready")

	notice := ctrl.ModelSetupNotice()
	require.NotEmpty(t, notice)
	assert.Contains(t, notice, catalog[0].Name, "the notice names the automatically picked model")
	assert.Contains(t, notice, "/model")

	// A fallback is not the user's pick: the remembered last model stays
	// untouched so the next session resolves it fresh.
	state, err := project.LoadUIState(proj.Global())
	require.NoError(t, err)
	assert.Empty(t, state.LastModel, "a catalog fallback must not become the remembered model")
}

func TestStartPromptRefusedWithoutModel(t *testing.T) {
	proj, cwd := newNoModelProject(t)
	bus := NewBus(nil)
	ctrl, err := NewController(bus, proj, cwd, "")
	require.NoError(t, err)
	t.Cleanup(ctrl.Close)

	ctrl.StartPrompt("hello", nil, "u1")

	assert.False(t, ctrl.RunActive(), "a refused submit must not start a turn")
	ctrl.streamMu.Lock()
	stopped := ctrl.streamRunning
	ctrl.streamMu.Unlock()
	assert.False(t, stopped, "no run loop may be in flight")

	// The user sees the same guidance the startup notice gave, and the
	// footer the submitter spun up is told the run is over.
	var sawHint, sawRunEnded bool
	deadline := time.After(time.Second)
	for !sawHint || !sawRunEnded {
		select {
		case <-bus.Chan():
			for _, msg := range bus.Drain() {
				switch m := msg.(type) {
				case SessionEventMsg:
					if up, ok := m.Event.(session.AssistantMessageUpdate); ok &&
						up.Message.State == session.StateError {
						assert.Contains(t, up.Message.Text, "/connect")
						assert.Contains(t, up.Message.Text, "/model")
						sawHint = true
					}
				case RunEndedMsg:
					sawRunEnded = true
				}
			}
		case <-deadline:
			t.Fatalf("refusal guidance missing: hint=%v runEnded=%v", sawHint, sawRunEnded)
		}
	}
}

// writeResumeSessionFile plants a minimal session jsonl whose header records
// model (empty for a session that never ran a turn).
func writeResumeSessionFile(t *testing.T, cwd, model string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "resume.jsonl")
	header := fmt.Sprintf(
		`{"type":"EntrySession","id":"resumemodel-01","timestamp":"2026-08-23T12:00:00Z","cwd":%q,"model":%q}`+"\n",
		cwd, model)
	require.NoError(t, os.WriteFile(path, []byte(header), 0o644))
	return path
}

// A resumed session that carries a model supplies it itself: the startup
// catalog fallback stays off, and its notice must not claim the pick.
func TestNewControllerResumeWithModelSkipsCatalogFallback(t *testing.T) {
	proj, cwd := newNoModelProject(t)
	connectOpenAI(t, proj)

	providers, err := provider.Open(provider.Options{
		CachePath:       proj.Global().ProviderCatalogFile(),
		CredentialsPath: proj.Global().CredentialsFile(),
	})
	require.NoError(t, err)
	catalog := providers.Models()
	require.NotEmpty(t, catalog, "the connected provider store must supply models")
	// The session runs the catalog's tail, so the test cannot pass by
	// accidentally hitting the first-model fallback.
	sessionModel := catalog[len(catalog)-1].Name

	ctrl, err := NewController(NewBus(nil), proj, cwd, writeResumeSessionFile(t, cwd, sessionModel))
	require.NoError(t, err)
	t.Cleanup(ctrl.Close)

	assert.Equal(t, sessionModel, ctrl.ModelName(), "the resumed session supplies the model")
	assert.Empty(t, ctrl.ModelSetupNotice(), "no catalog-fallback notice for a session's own model")
}

// A resume with no recorded model has nothing to supply, so the catalog
// fallback — and its notice — apply exactly like a fresh session.
func TestNewControllerResumeWithoutModelUsesCatalogFallback(t *testing.T) {
	proj, cwd := newNoModelProject(t)
	connectOpenAI(t, proj)

	ctrl, err := NewController(NewBus(nil), proj, cwd, writeResumeSessionFile(t, cwd, ""))
	require.NoError(t, err)
	t.Cleanup(ctrl.Close)

	catalog := ctrl.providers.Models()
	require.NotEmpty(t, catalog)
	assert.Equal(t, catalog[0].Name, ctrl.ModelName(), "the catalog fallback still applies")
	assert.Contains(t, ctrl.ModelSetupNotice(), "from the model catalog")
}
