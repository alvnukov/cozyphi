package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/project"
	"github.com/pulseaiclub/phi/internal/session"
)

// TestCompactEmptySessionSurfacesError pins the /compact feedback path: with
// nothing to summarize, the background run still reports back — as a session
// error event on the bus, the same channel stream errors use.
func TestCompactEmptySessionSurfacesError(t *testing.T) {
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

	bus := NewBus(nil)
	ctrl, err := NewController(bus, proj, cwd, "")
	require.NoError(t, err)

	ctrl.Compact()

	errText := ""
	deadline := time.Now().Add(2 * time.Second)
	for errText == "" && time.Now().Before(deadline) {
		for _, m := range bus.Drain() {
			ev, ok := m.(SessionEventMsg)
			if !ok {
				continue
			}
			if up, ok := ev.Event.(session.AssistantMessageUpdate); ok && up.Message.State == session.StateError {
				errText = up.Message.Text
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NotEmpty(t, errText, "expected a compaction error event on the bus")
	require.Contains(t, errText, "nothing to compact")
}
