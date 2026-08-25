package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/llm"
)

// TestEngineContextReportCombinesItemsAndStats: the browser view carries both
// the per-entry itemization and the aggregate window/threshold numbers.
func TestEngineContextReportCombinesItemsAndStats(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	require.NoError(t, engine.session.Append(
		llm.Message{Role: llm.RoleUser, Content: "question one"},
		llm.Message{Role: llm.RoleAssistant, Content: "answer one", Usage: llm.Usage{PromptTokens: 4242, TotalTokens: 4300}},
	))

	view := engine.ContextReport()

	require.Len(t, view.Items, 2)
	assert.Equal(t, "user", view.Items[0].Kind)
	assert.Equal(t, "question one", view.Items[0].Preview)
	assert.Equal(t, 100000, view.ContextWindow)
	assert.Equal(t, 100000-16384, view.ThresholdTokens)
	assert.Equal(t, 4242, view.ContextTokens)
	assert.Equal(t, "provider", view.TokenSource)
	assert.Positive(t, view.EstimatedTokens)
}

// TestEngineTrimContextFromRemovesEarlierEntriesFromView: trimming through
// the engine invalidates the cached context and the next view reflects it.
func TestEngineTrimContextFromRemovesEarlierEntriesFromView(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	require.NoError(t, engine.session.Append(
		llm.Message{Role: llm.RoleUser, Content: "q1"},
		llm.Message{Role: llm.RoleAssistant, Content: "a1"},
	))
	keep := engine.ContextReport().Items[1].EntryID

	require.NoError(t, engine.TrimContextFrom(keep))

	view := engine.ContextReport()
	require.Len(t, view.Items, 2)
	assert.Equal(t, "summary", view.Items[0].Kind)
	assert.Equal(t, keep, view.Items[1].EntryID)
	assert.True(t, view.LastCompaction.FromTrim)

	// The model's next request no longer contains the dropped message.
	msgs := engine.session.BuildContext()
	require.Len(t, msgs, 2)

	require.Error(t, engine.TrimContextFrom("nope"))
}
