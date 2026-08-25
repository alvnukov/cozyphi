package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// TestDropContextEntriesRemovesThemFromContext: deleting blocks appends a
// compaction entry carrying a drop mask; both BuildContext (the model's next
// request) and the browser view lose exactly the deleted messages and keep
// everything else, append-only.
func TestDropContextEntriesRemovesThemFromContext(t *testing.T) {
	m := newReportManager(t)
	u1 := appendMsg(t, m, llm.RoleUser, "q1")
	a1 := appendMsg(t, m, llm.RoleAssistant, "a1")
	u2 := appendMsg(t, m, llm.RoleUser, "q2")
	appendMsg(t, m, llm.RoleAssistant, "a2")

	require.NoError(t, m.DropContextEntries(a1, u2))

	report := m.InspectContext()
	ids := make([]string, 0, len(report.Items))
	for _, item := range report.Items {
		ids = append(ids, item.EntryID)
	}
	assert.NotContains(t, ids, a1, "deleted assistant entry leaves the view")
	assert.NotContains(t, ids, u2, "deleted user entry leaves the view")
	assert.Contains(t, ids, u1, "untouched entries stay")
	assert.Equal(t, "summary", report.Items[0].Kind, "the drop is recorded as a compaction entry")
	assert.True(t, report.LastCompaction.FromTrim, "user-initiated drops are marked like trims")

	msgs := m.BuildContext()
	var joined strings.Builder
	for _, e := range msgs {
		if me, ok := e.(SessionMessageEntry); ok {
			joined.WriteString(me.Message.Content + "|")
		}
	}
	assert.NotContains(t, joined.String(), "a1", "the model's next request omits deleted blocks")
	assert.NotContains(t, joined.String(), "q2")
	assert.Contains(t, joined.String(), "q1")
}

// TestDropAccumulatesAcrossDrops: a second drop must carry the first one's
// mask forward — each compaction entry fully describes the context shape, so
// the newest one has to remember older deletions.
func TestDropAccumulatesAcrossDrops(t *testing.T) {
	m := newReportManager(t)
	u1 := appendMsg(t, m, llm.RoleUser, "q1")
	a1 := appendMsg(t, m, llm.RoleAssistant, "a1")
	appendMsg(t, m, llm.RoleUser, "q2")
	appendMsg(t, m, llm.RoleAssistant, "a2")

	require.NoError(t, m.DropContextEntries(u1))
	require.NoError(t, m.DropContextEntries(a1))

	ids := contextIDs(m)
	assert.NotContains(t, ids, u1)
	assert.NotContains(t, ids, a1)
	assert.Len(t, ids, 3, "summary plus the two untouched messages")
}

// TestDropSurvivesTrim: trimming after a drop must not resurrect the deleted
// entries — the trim compaction carries the accumulated mask forward.
func TestDropSurvivesTrim(t *testing.T) {
	m := newReportManager(t)
	appendMsg(t, m, llm.RoleUser, "q1")
	a1 := appendMsg(t, m, llm.RoleAssistant, "a1")
	u2 := appendMsg(t, m, llm.RoleUser, "q2")
	appendMsg(t, m, llm.RoleAssistant, "a2")

	require.NoError(t, m.DropContextEntries(u2))
	_, err := m.TrimContextFrom(a1)
	require.NoError(t, err)

	ids := contextIDs(m)
	assert.NotContains(t, ids, u2, "trim must not resurrect deleted entries")
	assert.Contains(t, ids, a1)
}

// TestDropUnknownEntryFails: deleting an entry outside the session fails
// closed without touching the context.
func TestDropUnknownEntryFails(t *testing.T) {
	m := newReportManager(t)
	appendMsg(t, m, llm.RoleUser, "q1")

	require.Error(t, m.DropContextEntries("nope"))
	require.Error(t, m.DropContextEntries("nope", "also-nope"))
	assert.Len(t, m.InspectContext().Items, 1, "a failed drop changes nothing")
}

// TestInspectContextBodyCarriesFullText: the browser item keeps the block's
// full text for the detail popup, not just the one-line preview.
func TestInspectContextBodyCarriesFullText(t *testing.T) {
	m := newReportManager(t)
	appendMsg(t, m, llm.RoleUser, "first line\nsecond line\nthird line")

	report := m.InspectContext()
	require.Len(t, report.Items, 1)
	assert.Equal(t, "first line", report.Items[0].Preview)
	assert.Equal(t, "first line\nsecond line\nthird line", report.Items[0].Body)
}

func contextIDs(m *Manager) []string {
	report := m.InspectContext()
	ids := make([]string, 0, len(report.Items))
	for _, item := range report.Items {
		ids = append(ids, item.EntryID)
	}
	return ids
}
