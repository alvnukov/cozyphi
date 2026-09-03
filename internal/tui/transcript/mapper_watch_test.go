package transcript_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/watchtool"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

// watchRows indexes the transcript's watch tool rows by their detail.
func watchRows(entries []components.Widget) map[string]*block.ToolBlock {
	rows := map[string]*block.ToolBlock{}
	for _, e := range entries {
		if b, ok := e.(*block.ToolBlock); ok && b.Name == "watch" {
			rows[b.Detail] = b
		}
	}
	return rows
}

// The call that started a still-running watch pulses in the feed; the
// watch's log row, its fired events and the start row of an ended watch
// keep the plain checkmark. The live set is the mapper's, so a watch that
// ends flips its start row on the next sync.
func TestMapperMarksTheStartRowOfALiveWatch(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	live := []transcript.WatchRef{{ID: "w1", Label: "edge logs"}}
	m.LiveWatches = func() []transcript.WatchRef { return live }

	start := watchtool.StartDetail("w1", "edge logs")
	snap := session.Snapshot{
		Messages: []session.Message{
			{ID: "u1", Role: session.RoleUser, State: session.StateComplete, Text: "watch the logs"},
			{
				ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
				Content: []session.ContentBlock{
					{Type: session.BlockToolUse, ID: "t1", Name: "watch", Input: "start"},
					{Type: session.BlockToolUse, ID: "t2", Name: "watch", Input: "log"},
				},
			},
			{ID: "w1-1", Role: session.RoleWatch, Text: "edge logs"},
		},
		Tools: map[string]session.ToolRun{
			"t1": {ToolUseID: "t1", Name: "watch", Status: session.ToolDone, Detail: start, Output: "started"},
			"t2": {ToolUseID: "t2", Name: "watch", Status: session.ToolDone, Detail: "w1 (3)", Output: "3 events"},
			"w1-1": {
				ToolUseID: "w1-1",
				Name:      "watch",
				Status:    session.ToolDone,
				Detail:    "edge logs",
				Output:    "hit",
				Local:     true,
			},
		},
	}
	entries, ids, _ := m.Sync(nil, nil, snap)
	rows := watchRows(entries)
	require.Len(t, rows, 3)
	require.Equal(t, status.ToolLive, rows[start].Status, "the live watch's start row pulses")
	require.Equal(t, status.ToolDone, rows["w1 (3)"].Status, "the log row is an ordinary call")
	require.Equal(t, status.ToolDone, rows["edge logs"].Status, "a fired event is done")

	live = nil
	entries, _, dirty := m.Sync(entries, ids, snap)
	require.Equal(t, status.ToolDone, watchRows(entries)[start].Status, "an ended watch's start row settles")
	require.NotEmpty(t, dirty, "the settled row is redrawn")
}
