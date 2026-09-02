package transcript

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/tools/watchtool"
)

// ToggleWatches folds or unfolds every row a watch owns — its start call
// and its fired events — as one: any folded row opens them all, all open
// folds them all. Rows of another watch stay put, and a watch with no rows
// in the feed reports that instead of pretending.
func TestTranscriptPaneToggleWatchesFoldsTheWatchRowsTogether(t *testing.T) {
	pane := NewTranscriptPane(components.DefaultTheme(), nil, "test")
	th := components.DefaultTheme()
	start := &block.ToolBlock{
		Name:   "watch",
		Detail: watchtool.StartDetail("w1", "edge logs"),
		Output: "started",
		Theme:  th,
	}
	event := &block.ToolBlock{Name: "watch", Detail: "edge logs", Output: "hit", Theme: th}
	other := &block.ToolBlock{Name: "watch", Detail: watchtool.StartDetail("w2", "build"), Output: "started", Theme: th}
	pane.list.Entries = []components.Widget{start, event, other}
	pane.listIDs = []string{"a1-t1", "watch-w1-1", "a1-t3"}
	edge := []WatchRef{{ID: "w1", Label: "edge logs"}}

	require.True(t, pane.ToggleWatches(edge))
	require.True(t, start.Expanded && event.Expanded, "both rows of the watch open")
	require.False(t, other.Expanded, "another watch's row is untouched")
	require.Equal(t, "watch-w1-1", pane.revealID, "the last row of the watch is brought into view")

	const listH = 12
	pane.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: listH}}, 40, listH)
	require.Empty(t, pane.revealID, "the reveal is spent on the next frame")

	start.Expanded = false
	require.True(t, pane.ToggleWatches(edge))
	require.True(t, start.Expanded && event.Expanded, "a half-open watch opens fully")

	require.True(t, pane.ToggleWatches(edge))
	require.False(t, start.Expanded || event.Expanded, "a fully open watch folds")

	require.False(t, pane.ToggleWatches([]WatchRef{{ID: "w9", Label: "gone"}}), "no rows, nothing toggled")
}
