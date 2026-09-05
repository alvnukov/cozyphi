package sessions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

// subscriptionCredentials is a stored ChatGPT subscription: it puts the
// effort ladder on the openai catalog entries the way a live sign-in does.
const subscriptionCredentials = `{
	"version": 1,
	"providers": {
		"openai": {
			"type": "oauth",
			"access": "access",
			"refresh": "refresh",
			"expires": 4102444800000,
			"base_url": "https://chatgpt.com/backend-api/codex",
			"protocol": "openai-responses"
		}
	}
}`

// newEffortEditor builds the shell on a runtime catalog that offers effort
// levels, with no configured default model.
func newEffortEditor(t *testing.T) *View {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cwd := t.TempDir()

	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(proj.Global().CredentialsFile()), 0o755))
	require.NoError(t, os.WriteFile(proj.Global().CredentialsFile(), []byte(subscriptionCredentials), 0o600))

	bus := controller.NewBus(nil)
	ctrl, err := controller.NewController(bus, proj, cwd, "")
	require.NoError(t, err)
	e := NewView(nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "m", "", 0, nil, nil)
	e.SetActive(true)
	return e
}

// TestEditorEffortCommandRemoved: effort is chosen inside the model
// picker, so /effort must no longer dispatch to anything.
func TestEditorEffortCommandRemoved(t *testing.T) {
	e := newEffortEditor(t)
	require.NoError(t, e.SetModel("openai/gpt-5.5"))

	assert.False(t, e.commands.DispatchSlash("/effort high", e.commandContext()),
		"/effort must no longer dispatch")
}

// TestEditorSetModelEffortUpdatesLabel: one picker pick applies the model
// and its effort together, and the composer label names the effort while
// one is selected.
func TestEditorSetModelEffortUpdatesLabel(t *testing.T) {
	e := newEffortEditor(t)

	require.NoError(t, e.SetModelEffort("openai/gpt-5.5", "high"))
	assert.Equal(t, "openai/gpt-5.5 · high", e.composer.Chat.ModelLabel)

	require.NoError(t, e.SetModelEffort("openai/gpt-5.5", ""))
	assert.Equal(t, "openai/gpt-5.5", e.composer.Chat.ModelLabel,
		"clearing the effort must drop the suffix from the label")
}
