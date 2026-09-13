package sessions

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/shelltask"
	"github.com/alvnukov/cozyphi/internal/tui/shellpane"
)

func TestBackgroundShellAmbiguityOpensSelection(t *testing.T) {
	pane := shellpane.New(components.DefaultTheme(), nil, nil, nil)
	e := &View{shells: pane, shellSessionID: "current", shellTasks: []shelltask.Snapshot{
		{ID: "a", ParentSessionID: "current", State: shelltask.Running},
		{ID: "b", ParentSessionID: "current", State: shelltask.Running},
	}}
	e.BackgroundShell()
	require.True(t, pane.Visible(), "several foreground commands require explicit selection")
}

func TestForegroundShellSelectsOnlyCurrentConversation(t *testing.T) {
	tasks := []shelltask.Snapshot{
		{ID: "other", ParentSessionID: "other", State: shelltask.Running},
		{ID: "finished", ParentSessionID: "current", State: shelltask.Completed},
		{ID: "background", ParentSessionID: "current", State: shelltask.Running, Background: true},
		{ID: "current", ParentSessionID: "current", State: shelltask.Running},
	}
	id, count := foregroundShell(shellTasksForSession(tasks, "current"))
	require.Equal(t, "current", id)
	require.Equal(t, 1, count)
}
