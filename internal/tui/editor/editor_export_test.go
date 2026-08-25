package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

func newExportEditor(t *testing.T) (*Editor, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")

	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())

	bus := controller.NewBus(nil)
	ctrl, err := controller.NewController(bus, proj, cwd, "")
	require.NoError(t, err)

	e := NewEditor(nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "m", "", 0, nil, nil)
	return e, cwd
}

// TestExportSessionWritesMarkdown drives /export end to end: registry dispatch
// reaches the editor Host, which writes the transcript as markdown.
func TestExportSessionWritesMarkdown(t *testing.T) {
	e, cwd := newExportEditor(t)

	e.transcript.ApplySession(session.UserAppend{Text: "hello"})
	e.transcript.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
		ID:    "a1",
		State: session.StateComplete,
		Text:  "hi there",
	}})
	e.transcript.Sync()

	path := filepath.Join(cwd, "chat.md")
	require.True(t, e.commands.DispatchSlash("/export "+path, e.commandContext()))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "## User\n\nhello\n\n## Assistant\n\nhi there\n", string(data))
}

// TestEditorRegistersModelSlash: the editor assembly contributes /model
// with its configured names — the arg completer answers through the same
// registry the composer uses.
func TestEditorRegistersModelSlash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")

	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())

	bus := controller.NewBus(nil)
	ctrl, err := controller.NewController(bus, proj, cwd, "")
	require.NoError(t, err)

	e := NewEditor(nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "m", "", 0,
		[]string{"test-model", "other-model"}, nil)

	items := e.commands.FilterSlash("model")
	require.Len(t, items, 1, "editor registers /model for its model list")

	args, ok := e.commands.CompleteSlashArg("model", "oth")
	require.True(t, ok)
	require.Len(t, args, 1)
	assert.Equal(t, "other-model", args[0].Path)
}

// TestExportSessionDefaultPath pins the no-arg form: the file lands in the cwd
// named after the session id.
func TestExportSessionDefaultPath(t *testing.T) {
	e, cwd := newExportEditor(t)

	e.transcript.ApplySession(session.UserAppend{Text: "hello"})
	e.transcript.Sync()

	require.True(t, e.commands.DispatchSlash("/export", e.commandContext()))

	matches, err := filepath.Glob(filepath.Join(cwd, "cozyphi-*.md"))
	require.NoError(t, err)
	require.Len(t, matches, 1, "one exported markdown file in cwd")
}
