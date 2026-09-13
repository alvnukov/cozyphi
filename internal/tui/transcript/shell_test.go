package transcript

import (
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/shelltask"
)

func shellTranscript() *TranscriptPane {
	p := NewTranscriptPane(components.DefaultTheme(), nil, "")
	p.ApplySession(session.UserAppend{ID: "u1", Text: "run"})
	p.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
		ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
		Content: []session.ContentBlock{{Type: session.BlockToolUse, ID: "call1", Name: "bash", Input: "make"}},
	}})
	p.Sync()
	return p
}

func shellScreen(p *TranscriptPane) string {
	return components.SurfaceText(p.Draw(components.DrawContext{
		Max: components.Size{Width: 120, Height: 35}, Method: xui.WidthUnicode,
	}, 120, 35))
}

func TestShellSnapshotOverridesLateLaunchReceipt(t *testing.T) {
	deadline := time.Date(2026, 9, 13, 12, 30, 0, 0, time.UTC)
	exitCode := 7
	p := shellTranscript()
	task := shelltask.Snapshot{
		ID: "s1", ToolUseID: "call1", Command: "make", State: shelltask.Running, Background: true,
		Output: "in progress", Deadline: &deadline,
	}
	p.SetShellTasks([]shelltask.Snapshot{task})
	p.Sync()
	require.Contains(t, shellScreen(p), "(background)")
	require.Contains(t, shellScreen(p), "expires 12:30:00")

	task.State, task.ExitCode, task.Output = shelltask.Failed, &exitCode, "build failed"
	p.SetShellTasks([]shelltask.Snapshot{task})
	p.Sync()
	require.Contains(t, shellScreen(p), "(failed)")
	require.Contains(t, shellScreen(p), "exit code: 7")

	p.ApplySession(session.ToolData{Run: session.ToolRun{
		ToolUseID: "call1", Name: "bash", Status: session.ToolDone,
		Detail: "make", Output: "background task launched",
	}})
	p.Sync()
	screen := shellScreen(p)
	require.Contains(t, screen, "(failed)")
	require.Contains(t, screen, "exit code: 7")
	require.NotContains(t, screen, "(background)")
	require.NotContains(t, screen, "background task launched")
}

func TestShellSnapshotUpdatesEarlierTurnAndKeepsItVisible(t *testing.T) {
	p := shellTranscript()
	for _, id := range []string{"u2", "u3", "u4"} {
		p.ApplySession(session.UserAppend{ID: id, Text: "next"})
		p.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
			ID: "answer" + id, Role: session.RoleAssistant, State: session.StateComplete, Text: "done",
		}})
	}
	p.Sync()
	p.SetShellTasks(
		[]shelltask.Snapshot{
			{
				ID:         "s1",
				ToolUseID:  "call1",
				Command:    "make",
				State:      shelltask.Running,
				Background: true,
				Output:     "still working",
			},
		},
	)
	p.Sync()
	require.Contains(t, shellScreen(p), "(background)")
}

func TestShellStoppedSnapshotShowsStopped(t *testing.T) {
	p := shellTranscript()
	p.SetShellTasks(
		[]shelltask.Snapshot{
			{ID: "s1", ToolUseID: "call1", Command: "make", State: shelltask.Stopped, Background: true},
		},
	)
	p.Sync()
	require.Contains(t, shellScreen(p), "(stopped)")
}
