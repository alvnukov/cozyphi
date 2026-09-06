package commands

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

func TestSessionListShowsTitleAndOwner(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	proj, err := project.Discover(t.TempDir())
	require.NoError(t, err)
	ctrl, err := controller.NewController(controller.NewBus(nil), proj, proj.Root(), "")
	require.NoError(t, err)
	t.Cleanup(ctrl.Close)
	require.NoError(t, ctrl.SetSessionTitle("named conversation"))
	pane := transcript.NewTranscriptPane(components.DefaultTheme(), nil, "test")
	cmd := &SessionCommands{Ctrl: ctrl, Transcript: pane}
	cmd.Show()
	snapshot := pane.Snapshot()
	require.NotEmpty(t, snapshot.Messages)
	text := snapshot.Messages[len(snapshot.Messages)-1].Text
	require.Contains(t, text, "named conversation")
	require.Contains(t, text, session.ShortID(ctrl.SessionID())+" [active]")
}
