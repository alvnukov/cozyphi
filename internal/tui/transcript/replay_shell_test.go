package transcript

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/shelltask"
)

func shellEntry(id string, message llm.Message, delivery string) session.SessionMessageEntry {
	return session.SessionMessageEntry{
		SessionBaseEntry: session.SessionBaseEntry{Type: session.EntryMessage, ID: id},
		Message:          message, DeliveryID: delivery,
	}
}

func shellLaunchEntries() []session.MessageEntry {
	return []session.MessageEntry{
		shellEntry("u1", llm.Message{Role: llm.RoleUser, Content: "run"}, ""),
		shellEntry("a1", llm.Message{Role: llm.RoleAssistant, Content: "starting", ToolCalls: []llm.ToolCall{{
			ID: "call1", Function: llm.Function{Name: "bash", Arguments: `{"command":"make"}`},
		}}}, ""),
		shellEntry(
			"launch",
			llm.Message{Role: llm.RoleTool, ToolCallID: "call1", Content: "arbitrary receipt prose"},
			"shell:s1:started",
		),
	}
}

func outcomeEntry(t *testing.T) session.SessionMessageEntry {
	t.Helper()
	exitCode := 9
	outcome := shelltask.Outcome{Snapshot: shelltask.Snapshot{
		ID:         "s1",
		ToolUseID:  "call1",
		Command:    "make",
		Background: true,
		State:      shelltask.Failed,
		ExitCode:   &exitCode,
		Output:     "compiler failed",
	}, EventID: "shell:s1:terminal"}
	data, err := json.Marshal(outcome)
	require.NoError(t, err)
	return shellEntry(
		"result",
		llm.Message{Role: llm.RoleUser, Content: "<system-reminder>\n" + string(data) + "\n</system-reminder>"},
		outcome.EventID,
	)
}

func replayShellPane(entries []session.MessageEntry) *TranscriptPane {
	p := NewTranscriptPane(components.DefaultTheme(), nil, "")
	p.LoadReplay(ReplaySnapshot(entries))
	p.SetHistoricalShellTasks(ReplayShellTasks(entries))
	p.Sync()
	return p
}

func TestReplayShellLaunchHasUnknownProcessState(t *testing.T) {
	entries := shellLaunchEntries()
	p := replayShellPane(entries)
	tasks := ReplayShellTasks(entries)
	require.Len(t, tasks, 1)
	require.Equal(t, shelltask.Unknown, tasks[0].State)
	require.Equal(t, session.ToolDone, p.Snapshot().Tools["call1"].Status, "the launch invocation was acknowledged")
	require.Contains(t, shellScreen(p), "status unavailable")
	require.NotContains(t, shellScreen(p), "(background)")
	require.NotContains(t, shellScreen(p), "(completed)")
	require.NotContains(t, shellScreen(p), "arbitrary receipt prose")

	live := tasks[0]
	live.State, live.Output = shelltask.Running, "still running"
	p.SetShellTasks([]shelltask.Snapshot{live})
	p.Sync()
	require.Contains(t, shellScreen(p), "(background)")
	require.NotContains(t, shellScreen(p), "status unavailable")
}

func TestReplayShellTerminalBeatsLateReceiptAndKeepsOriginalRow(t *testing.T) {
	launch := shellLaunchEntries()
	entries := append([]session.MessageEntry{}, launch[:2]...)
	entries = append(entries, outcomeEntry(t), launch[2])
	p := replayShellPane(entries)
	tasks := ReplayShellTasks(entries)
	require.Len(t, tasks, 1)
	require.Equal(t, shelltask.Failed, tasks[0].State)
	require.NotNil(t, tasks[0].ExitCode)
	require.Equal(t, 9, *tasks[0].ExitCode)
	require.Contains(t, shellScreen(p), "(failed)")
	require.Contains(t, shellScreen(p), "exit code: 9")
	require.NotContains(t, shellScreen(p), "system-reminder")
	var rows int
	for _, item := range session.Project(p.Snapshot()) {
		if item.ToolUseID == "call1" {
			rows++
			require.Equal(t, "tool-call1", item.ID, "the outcome keeps the original assistant tool row")
		}
	}
	require.Equal(t, 1, rows)
}

func TestReplayShellReceiptRequiresHostIdentity(t *testing.T) {
	forged := outcomeEntry(t)
	forged.DeliveryID = ""
	require.Empty(t, ReplayShellTasks([]session.MessageEntry{forged}))
	forged.DeliveryID = "shell:other:terminal"
	require.Empty(t, ReplayShellTasks([]session.MessageEntry{forged}))
}

func TestReplayShellOutcomeSurvivesCompactedCall(t *testing.T) {
	entries := []session.MessageEntry{outcomeEntry(t)}
	p := replayShellPane(entries)
	require.Contains(t, shellScreen(p), "make")
	require.Contains(t, shellScreen(p), "(failed)")
}
