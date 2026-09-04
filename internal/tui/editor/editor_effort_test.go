package editor

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
func newEffortEditor(t *testing.T) *Editor {
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
	return NewEditor(nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "m", "", 0, nil, nil)
}

// TestEditorEffortCommandAndLabel: /effort is registered with the active
// model's levels behind "default", and the composer label names the effort
// while one is selected.
func TestEditorEffortCommandAndLabel(t *testing.T) {
	e := newEffortEditor(t)
	require.NoError(t, e.SetModel("openai/gpt-5.5"))

	items, ok := e.commands.CompleteSlashArg("effort", nil, "")
	require.True(t, ok, "/effort must offer argument completion")
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.Path)
	}
	assert.Equal(t, []string{"default", "minimal", "low", "medium", "high"}, paths)

	require.NoError(t, e.SetEffort("high"))
	assert.Equal(t, "openai/gpt-5.5 · high", e.composer.Chat.ModelLabel)

	require.True(t, e.commands.DispatchSlash("/effort default", e.commandContext()))
	assert.Equal(t, "openai/gpt-5.5", e.composer.Chat.ModelLabel,
		"clearing the effort must drop the suffix from the label")
}
