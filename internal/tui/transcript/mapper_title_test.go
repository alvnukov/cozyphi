package transcript_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

func TestTitleRowLiveAndReplay(t *testing.T) {
	snap := session.Snapshot{
		Messages: []session.Message{
			{
				ID:    "a1",
				Role:  session.RoleAssistant,
				State: session.StateComplete,
				Content: []session.ContentBlock{
					{Type: session.BlockToolUse, ID: "t1", Name: "session", Input: "untrusted raw input"},
				},
			},
		},
		Tools: map[string]session.ToolRun{"t1": {ToolUseID: "t1", Name: "session", Status: session.ToolInProgress}},
	}
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	entries, ids, _ := m.Sync(nil, nil, snap)
	check := func(entries []components.Widget, want string) {
		t.Helper()
		var row *block.ToolBlock
		for _, entry := range entries {
			if b, ok := entry.(*block.ToolBlock); ok {
				row = b
			}
		}
		require.NotNil(t, row)
		require.Equal(t, "Title", row.Name)
		require.Equal(t, want, row.Detail)
		require.Empty(t, row.Output)
		require.Nil(t, row.OnToggle)
	}
	check(entries, "set_title")
	snap.Tools["t1"] = session.ToolRun{
		ToolUseID: "t1",
		Name:      "session",
		Status:    session.ToolDone,
		Detail:    "Имена сессий",
		Output:    "Title: Имена сессий",
	}
	entries, _, _ = m.Sync(entries, ids, snap)
	check(entries, "Имена сессий")
	replay := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	entries, _, _ = replay.Sync(nil, nil, snap)
	check(entries, "Имена сессий")
}
