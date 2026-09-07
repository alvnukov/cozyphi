package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// overviewOf is one category's place in an answer.
func overviewOf(t *testing.T, snapshot diag.Snapshot, category diag.Category) diag.CategorySnapshot {
	t.Helper()
	for _, entry := range snapshot.Categories {
		if entry.Category == category {
			return entry
		}
	}
	t.Fatalf("the answer has no %s category", category)
	return diag.CategorySnapshot{}
}

// The overview over the real wiring, which is the only place the question is
// really settled: eleven collectors, all of them implemented, all of them
// wanting more room than the answer has. Every one of them has to come back
// with something, because a category that says nothing is indistinguishable
// from a category that found nothing.
//
// The counts are logged rather than pinned. What each collector has to say
// changes with every ticket, and a test that failed on that would be
// measuring the collectors, not the budget.
func TestEveryImplementedCategoryGetsAShareOfTheOverview(t *testing.T) {
	c := developerToolRuntime(t)

	overview, err := c.diagnostics.Snapshot(t.Context(), "")
	require.NoError(t, err)
	require.Len(t, overview.Categories, len(diag.Categories()))

	for _, entry := range overview.Categories {
		body, err := json.Marshal(entry)
		require.NoError(t, err)
		t.Logf("%-13s rows=%2d bytes=%5d %s truncated=%v",
			entry.Category, len(entry.Overview), len(body), entry.Availability, entry.Truncated)
		if entry.Availability != diag.AvailabilityAvailable {
			continue
		}
		assert.NotEmpty(t, entry.Overview,
			"%s can be observed and still answered nothing", entry.Category)
	}
	t.Logf("overview rendered=%d truncated=%v partial=%v",
		len(overview.JSON()), overview.Truncated, overview.Partial)
	assert.Equal(t, overview.Truncated, overview.Note != "",
		"an answer that had to leave something out says so")
}

// The note tells a reader to narrow to one category, so every category has to
// be worth narrowing to. This is the promise the shared budget is allowed to
// break and the detail answer is not.
func TestEveryImplementedCategoryFitsItsOwnAnswer(t *testing.T) {
	c := developerToolRuntime(t)

	for _, category := range diag.Categories() {
		detail, err := c.diagnostics.Snapshot(t.Context(), category)
		require.NoError(t, err)
		require.Len(t, detail.Categories, 1)
		if detail.Categories[0].Availability != diag.AvailabilityAvailable {
			continue
		}
		assert.False(t, detail.Truncated, "%s does not fit an answer of its own", category)
		assert.NotEmpty(t, detail.Categories[0].Fields)
	}
}
