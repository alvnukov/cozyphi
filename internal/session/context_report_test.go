package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func newReportManager(t *testing.T) *Manager {
	t.Helper()
	return NewManager(t.TempDir())
}

func appendMsg(t *testing.T, m *Manager, role llm.Role, content string) string {
	t.Helper()
	var (
		id  string
		err error
	)
	if role == llm.RoleAssistant {
		id, err = m.AppendAssistant(llm.Message{Role: role, Content: content}, "glm-5.2")
	} else {
		id, err = m.Append(llm.Message{Role: role, Content: content})
	}
	require.NoError(t, err)
	return id
}

// TestInspectContextItemizesCurrentContext: the report mirrors BuildContext
// entry by entry — a compaction leads as a summary item, each message carries
// its role, a one-line preview, and monotonically growing cumulative tokens.
func TestInspectContextItemizesCurrentContext(t *testing.T) {
	m := newReportManager(t)
	appendMsg(t, m, llm.RoleUser, "first question")
	a1 := appendMsg(t, m, llm.RoleAssistant, "first answer")
	appendMsg(t, m, llm.RoleUser, "second question\nwith detail")
	appendMsg(t, m, llm.RoleAssistant, "second answer")

	cmpID, err := m.AppendCompaction(Compaction{
		Summary:          "old stuff",
		FirstKeptEntryID: a1,
	})
	require.NoError(t, err)
	after := appendMsg(t, m, llm.RoleUser, "after compaction")

	report := m.InspectContext()

	// Path: u1, a1, u2, a2, cmp, after; FirstKeptEntryID=a1 keeps a1..after.
	require.Len(t, report.Items, 5)
	assert.Equal(t, "summary", report.Items[0].Kind)
	assert.Equal(t, cmpID, report.Items[0].EntryID)
	assert.Equal(t, "old stuff", report.Items[0].Preview)
	assert.Equal(t, a1, report.Items[1].EntryID)
	assert.Equal(t, "assistant", report.Items[1].Kind)
	assert.Equal(t, after, report.Items[len(report.Items)-1].EntryID)
	assert.Equal(t, "second question", report.Items[2].Preview, "preview is the first line")

	for i, item := range report.Items {
		assert.GreaterOrEqual(t, item.Tokens, 1, "item %d", i)
	}
	for i := 1; i < len(report.Items); i++ {
		assert.Greater(t, report.Items[i].CumulativeTokens, report.Items[i-1].CumulativeTokens)
	}
	assert.Equal(t, report.Items[len(report.Items)-1].CumulativeTokens, report.EstimatedTokens)

	assert.NotNil(t, report.LastCompaction)
	assert.Equal(t, "old stuff", report.LastCompaction.Summary)
}

func TestInspectContextEmpty(t *testing.T) {
	m := newReportManager(t)
	report := m.InspectContext()
	assert.Empty(t, report.Items)
	assert.Equal(t, 0, report.EstimatedTokens)
	assert.Nil(t, report.LastCompaction)
}

func TestInspectContextPreviewTruncates(t *testing.T) {
	m := newReportManager(t)
	appendMsg(t, m, llm.RoleUser, strings.Repeat("x", 300))
	report := m.InspectContext()
	require.Len(t, report.Items, 1)
	assert.Len(t, report.Items[0].Preview, previewWidth)
}

// TestTrimContextFrom: trimming appends a compaction entry that keeps exactly
// the selected entry onward, without an LLM summary, and reports it as a trim.
func TestTrimContextFrom(t *testing.T) {
	m := newReportManager(t)
	appendMsg(t, m, llm.RoleUser, "q1")
	keep := appendMsg(t, m, llm.RoleAssistant, "a1")
	appendMsg(t, m, llm.RoleUser, "q2")
	appendMsg(t, m, llm.RoleAssistant, "a2")

	cmpID, err := m.TrimContextFrom(keep)
	require.NoError(t, err)
	require.NotEmpty(t, cmpID)

	report := m.InspectContext()
	require.Len(t, report.Items, 4, "summary + kept a1 onward (u2, a2 stay)")
	assert.Equal(t, "summary", report.Items[0].Kind)
	assert.Equal(t, cmpID, report.Items[0].EntryID)
	assert.True(t, report.LastCompaction.FromTrim)
	assert.Equal(t, keep, report.Items[1].EntryID)
	assert.NotEmpty(t, report.Items[0].Preview, "trim note stands in for the summary")

	// Trimming from an unknown entry fails closed.
	_, err = m.TrimContextFrom("nope")
	assert.Error(t, err)
}

// TestEstimateMessageTokensShared: the estimator is the single source every
// token number in the UI and compaction shares.
func TestEstimateMessageTokensShared(t *testing.T) {
	n := EstimateMessageTokens(llm.Message{Role: llm.RoleUser, Content: "hello"})
	assert.GreaterOrEqual(t, n, 1)
	assert.GreaterOrEqual(t, EstimateMessageTokens(llm.Message{}), 1)
}
