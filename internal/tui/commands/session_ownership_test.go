package commands

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

func TestSessionListMarksActiveOwner(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	proj, err := project.Discover(t.TempDir())
	require.NoError(t, err)
	c, err := controller.NewController(controller.NewBus(nil), proj, proj.Root(), "")
	require.NoError(t, err)
	t.Cleanup(c.Close)
	m, err := session.NewSessionManager(
		proj.Root(),
		session.WithSessionDir(c.SessionDir()),
		session.WithShouldFlush(true),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	_, err = m.Append(llm.Message{Role: llm.RoleAssistant, Content: "saved"})
	require.NoError(t, err)
	pane := transcript.NewTranscriptPane(components.DefaultTheme(), nil, "test")
	commands := &SessionCommands{Ctrl: c, Transcript: pane}
	commands.Show()
	snapshot := pane.Snapshot()
	require.NotEmpty(t, snapshot.Messages)
	require.Contains(t, snapshot.Messages[len(snapshot.Messages)-1].Text, "[active]")
	require.NoError(t, m.Close())
	commands.Show()
	snapshot = pane.Snapshot()
	require.NotContains(t, snapshot.Messages[len(snapshot.Messages)-1].Text, "[active]")
}
