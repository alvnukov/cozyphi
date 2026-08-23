package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/agent"
	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/project"
	"github.com/pulseaiclub/phi/internal/tui/controller"
)

// TestEditorModeToggleWiring: the composer's Tab publishes ModeToggleMsg; the
// editor routes it to the controller and repaints the posture label.
func TestEditorModeToggleWiring(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PHI_MODEL", "test-model")
	t.Setenv("PHI_API_KEY", "test-key")
	t.Setenv("PHI_BASE_URL", "http://127.0.0.1:9")

	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())

	bus := controller.NewBus(nil)
	ctrl, err := controller.NewController(bus, proj, cwd, "")
	require.NoError(t, err)

	e := NewEditor(nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "m", "", 0, nil)
	assert.Equal(t, "⏵⏵ build", e.composer.Chat.TopLeftLabel.Text, "startup label")

	e.Update(controller.ModeToggleMsg{})
	assert.Equal(t, agent.ModePlan, ctrl.Mode())
	assert.Equal(t, "⏵⏵ plan", e.composer.Chat.TopLeftLabel.Text)

	e.Update(controller.ModeToggleMsg{})
	assert.Equal(t, agent.ModeBuild, ctrl.Mode())
	assert.Equal(t, "⏵⏵ build", e.composer.Chat.TopLeftLabel.Text)
}
