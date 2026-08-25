package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// TestEngineDropContextEntriesRemovesFromView: dropping through the engine
// invalidates the cached context and the next view reflects the deletions.
func TestEngineDropContextEntriesRemovesFromView(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	require.NoError(t, engine.session.Append(
		llm.Message{Role: llm.RoleUser, Content: "q1"},
		llm.Message{Role: llm.RoleAssistant, Content: "a1"},
		llm.Message{Role: llm.RoleUser, Content: "q2"},
	))
	drop := engine.ContextReport().Items[1].EntryID

	require.NoError(t, engine.DropContextEntries([]string{drop}))

	view := engine.ContextReport()
	require.Len(t, view.Items, 3, "summary plus the two survivors")
	for _, item := range view.Items {
		assert.NotEqual(t, drop, item.EntryID)
	}

	require.Error(t, engine.DropContextEntries([]string{"nope"}))
}
