package session

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func TestNoCompactionIsSaidRatherThanShownAsZeroes(t *testing.T) {
	m := newReportManager(t)
	appendMsg(t, m, llm.RoleUser, "first question")
	appendMsg(t, m, llm.RoleAssistant, "first answer")

	stats := m.CompactionStats()

	assert.Zero(t, stats.Count)
	assert.False(t, stats.Known,
		"the counters below are zero because none ever ran, not because one freed nothing")
}

func TestCompactionStatsReportTheLatestCompactionOnThePath(t *testing.T) {
	m := newReportManager(t)
	appendMsg(t, m, llm.RoleUser, "first question")
	a1 := appendMsg(t, m, llm.RoleAssistant, "first answer")
	appendMsg(t, m, llm.RoleUser, "second question")

	_, err := m.AppendCompaction(Compaction{
		Summary:            "SENTINEL-SUMMARY: everything the user said so far",
		FirstKeptEntryID:   a1,
		TokensBefore:       9000,
		TokensAfter:        1200,
		MessagesSummarized: 6,
		MessagesKept:       2,
	})
	require.NoError(t, err)
	appendMsg(t, m, llm.RoleUser, "after compaction")

	stats := m.CompactionStats()

	assert.Equal(t, 1, stats.Count)
	assert.True(t, stats.Known)
	assert.False(t, stats.FromTrim)
	assert.Equal(t, 9000, stats.TokensBefore)
	assert.Equal(t, 1200, stats.TokensAfter)
	assert.Equal(t, 6, stats.MessagesSummarized)
	assert.Equal(t, 2, stats.MessagesKept)

	rendered, err := json.Marshal(stats)
	require.NoError(t, err)
	assert.NotContains(t, string(rendered), "SENTINEL-SUMMARY",
		"the summary describes the user's work and never leaves the manager through here")
	assert.NotContains(t, string(rendered), a1, "nor do the entry ids it kept")
}

func TestATrimIsCountedAsTheCompactionItIs(t *testing.T) {
	m := newReportManager(t)
	appendMsg(t, m, llm.RoleUser, "first question")
	a1 := appendMsg(t, m, llm.RoleAssistant, "first answer")

	_, err := m.AppendCompaction(Compaction{
		FirstKeptEntryID: a1,
		FromTrim:         true,
		MessagesKept:     1,
	})
	require.NoError(t, err)

	stats := m.CompactionStats()

	assert.Equal(t, 1, stats.Count)
	assert.True(t, stats.Known)
	assert.True(t, stats.FromTrim, "the user cut history by hand; nothing was summarized")
	assert.Zero(t, stats.MessagesSummarized)
}

func TestAskingForCompactionStatsCompactsNothing(t *testing.T) {
	m := newReportManager(t)
	appendMsg(t, m, llm.RoleUser, "first question")
	appendMsg(t, m, llm.RoleAssistant, "first answer")
	before := len(m.BuildContext())

	for range 3 {
		m.CompactionStats()
	}

	assert.Len(t, m.BuildContext(), before,
		"a read of the path appends nothing to it")
	assert.Zero(t, m.CompactionStats().Count)
}

func TestANilManagerAnswersEmptyCompactionStats(t *testing.T) {
	var m *Manager
	assert.Equal(t, CompactionStats{}, m.CompactionStats())
}
