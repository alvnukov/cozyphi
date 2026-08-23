package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/project"
	"github.com/pulseaiclub/phi/internal/tui/controller"
)

// TestNewEditorShowsResumedHistory covers `phi --continue/--resume`: when the
// controller boots on an existing session, the transcript already carries the
// replay before the first frame renders.
func TestNewEditorShowsResumedHistory(t *testing.T) {
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

	sessionDir := proj.SessionDir()
	require.NoError(t, os.MkdirAll(sessionDir, 0o755))
	path := filepath.Join(sessionDir, "sess_editorres-0001.jsonl")
	content := `{"type":"EntrySession","id":"editorres-0001","timestamp":"2026-08-23T12:00:00Z","cwd":"/tmp"}` + "\n" +
		`{"type":"EntryMessage","id":"m1","message":{"role":"user","content":"hello editor"}}` + "\n" +
		`{"type":"EntryMessage","id":"m2","parentID":"m1","message":{"role":"assistant","content":"visible on first frame"}}` + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	bus := controller.NewBus(nil)
	ctrl, err := controller.NewController(bus, proj, cwd, path)
	require.NoError(t, err)

	e := NewEditor(nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "m", "", 0, nil)
	snap := e.transcript.Snapshot()
	require.Len(t, snap.Messages, 2)
	assert.Equal(t, "hello editor", snap.Messages[0].Text)
	assert.Equal(t, "visible on first frame", snap.Messages[1].Text)
}

// TestNewEditorFreshSessionKeepsEmptyTranscript pins the bare-`phi` behavior:
// no replay is loaded when the controller started a new session.
func TestNewEditorFreshSessionKeepsEmptyTranscript(t *testing.T) {
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
	assert.Empty(t, e.transcript.Snapshot().Messages)
}
