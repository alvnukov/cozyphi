package sessions

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

func TestTitleToastOnlyOnSuccessfulLiveCall(t *testing.T) {
	for _, status := range []session.ToolStatus{session.ToolInProgress, session.ToolError, session.ToolRejected, session.ToolDone} {
		e := newTestEditor(t)
		e.bus.Publish(
			controller.SessionEventMsg{
				Event: session.ToolData{
					Run: session.ToolRun{ToolUseID: "title", Name: "session", Status: status, Detail: "Имена сессий"},
				},
			},
		)
		e.drainBus()
		if status == session.ToolDone {
			require.Equal(t, "Session named: Имена сессий", e.toast.Message)
			require.True(t, e.toast.Visible())
		} else {
			require.False(t, e.toast.Visible())
		}
	}
}
