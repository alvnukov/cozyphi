package diag_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// bulky is a collector with far more to say than one category's share of an
// overview can hold. Eleven of them is a catalog that cannot fit by any
// arrangement of the budget, which is the only shape in which the question
// this file asks — who goes without — has an answer at all.
func bulky(category diag.Category, fields int) *fake {
	collector := &fake{category: category}
	keys := make([]string, 0, fields)
	for i := range fields {
		key := fmt.Sprintf("field-%02d", i)
		keys = append(keys, key)
		collector.fields = append(collector.fields, stringField(key, "value-00"))
	}
	collector.status = available(keys...)
	return collector
}

// crowdedFields is more than one category's share of the default budget can
// hold and less than a whole detail answer can, so one fixture stands for
// both halves of the contract: an overview nobody fits into and a detail view
// everybody fits into.
const crowdedFields = 40

// crowded is a registry whose every category has more than it can say.
func crowded() *diag.Registry {
	collectors := make([]diag.Collector, 0, len(diag.Categories()))
	for _, category := range diag.Categories() {
		collectors = append(collectors, bulky(category, crowdedFields))
	}
	return diag.NewRegistry(fixedClock(), diag.DefaultLimits(), collectors...)
}

// rowsPerCategory is how much each category got to say, in catalog order.
func rowsPerCategory(snapshot diag.Snapshot) []int {
	rows := make([]int, 0, len(snapshot.Categories))
	for _, entry := range snapshot.Categories {
		rows = append(rows, len(entry.Overview))
	}
	return rows
}

// The tail of the catalog is where every new category of this epic lands, and
// under a single running total it was also where the budget had already run
// out. A category that answers nothing because of where it sits in a list is
// not reporting anything about the process — it is reporting the list.
func TestNoCategoryIsEmptyBecauseOfWhereItSitsInTheCatalog(t *testing.T) {
	snapshot, err := crowded().Snapshot(t.Context(), "")
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, len(diag.Categories()))

	rows := rowsPerCategory(snapshot)
	for i, entry := range snapshot.Categories {
		assert.NotEmpty(t, entry.Overview, "%s answered nothing at all", entry.Category)
		assert.Positive(t, rows[i])
	}

	// Every category here has identical fields, so identical shares must buy
	// identical numbers of rows. The one row of slack is integer division:
	// what a category leaves behind enlarges the shares after it.
	assert.LessOrEqual(t, slices.Max(rows)-slices.Min(rows), 1,
		"categories with the same amount to say get the same amount of room")
	assert.GreaterOrEqual(t, rows[len(rows)-1], rows[0],
		"the last category in the catalog is not the one that pays for the rest")
}

// The share is of what is left, not of what the answer started with, so a
// category that had little to say hands the rest on instead of locking it
// away. Otherwise an equal division would be its own kind of waste.
func TestWhatOneCategoryDoesNotSpendIsLeftForTheOthers(t *testing.T) {
	collectors := make([]diag.Collector, 0, len(diag.Categories()))
	for i, category := range diag.Categories() {
		if i == 0 {
			collectors = append(collectors, bulky(category, 1))
			continue
		}
		collectors = append(collectors, bulky(category, crowdedFields))
	}
	generous, err := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), collectors...).
		Snapshot(t.Context(), "")
	require.NoError(t, err)

	even, err := crowded().Snapshot(t.Context(), "")
	require.NoError(t, err)

	require.Len(t, generous.Categories[0].Overview, 1, "the first category said all it had")
	assert.Greater(t, len(generous.Categories[1].Overview), len(even.Categories[1].Overview),
		"the room the first category did not need went to the ones after it")
}

// A budget that ran out is a fact about the answer, and the answer says it in
// both places a reader looks: the category that was cut and the note that
// tells them what to do about it.
func TestAnOverviewThatRanOutOfRoomSaysSoRatherThanAnsweringLess(t *testing.T) {
	snapshot, err := crowded().Snapshot(t.Context(), "")
	require.NoError(t, err)

	assert.True(t, snapshot.Truncated)
	assert.Contains(t, snapshot.Note, "narrow it with category")
	for _, entry := range snapshot.Categories {
		assert.True(t, entry.Truncated, "%s was cut short and says so", entry.Category)
	}
}

// Two snapshots of the same process are meant to be comparable, which they
// are not if the budget can reorder them. Sharing decides how much each
// category says, never where it says it.
func TestSharingTheBudgetLeavesTheCatalogOrderAlone(t *testing.T) {
	snapshot, err := crowded().Snapshot(t.Context(), "")
	require.NoError(t, err)

	order := make([]diag.Category, 0, len(snapshot.Categories))
	for _, entry := range snapshot.Categories {
		order = append(order, entry.Category)
	}
	assert.Equal(t, diag.Categories(), order)

	again, err := crowded().Snapshot(t.Context(), "")
	require.NoError(t, err)
	assert.Equal(t, rowsPerCategory(snapshot), rowsPerCategory(again),
		"the same process answered twice divides the budget the same way")
}

// Narrowing to one category is what the truncation note tells a reader to do,
// so it has to be worth doing: a category asked for by name is the only claim
// on the budget and answers in full, layers and provenance included.
func TestACategoryAskedForByNameGetsTheWholeBudget(t *testing.T) {
	registry := crowded()
	overview, err := registry.Snapshot(t.Context(), "")
	require.NoError(t, err)

	for _, category := range diag.Categories() {
		detail, err := registry.Snapshot(t.Context(), category)
		require.NoError(t, err)
		require.Len(t, detail.Categories, 1)
		assert.False(t, detail.Truncated, "%s does not fit its own answer", category)
		assert.Len(t, detail.Categories[0].Fields, crowdedFields)
		assert.Greater(t, len(detail.Categories[0].Fields), len(entryFor(t, overview, category).Overview),
			"the detail view is what the reader was sent to it for")
	}
}

// An overview row is a state and a value against a key. Provenance is what
// makes a row expensive — a ref is a path or a config key — and it is exactly
// what explain exists to answer, so the row does not carry it and the budget
// is not charged for it.
func TestAnOverviewRowCarriesTheValueAndNotWhereItCameFrom(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), &fake{
		category: diag.CategoryRuntime,
		status:   available("mode"),
		fields:   []diag.Field{stringField("mode", "headless")},
	})

	snapshot, err := registry.Snapshot(t.Context(), "")
	require.NoError(t, err)
	row := entryFor(t, snapshot, diag.CategoryRuntime).Overview
	require.Len(t, row, 1)
	assert.Equal(t, "mode", row[0].Key)
	assert.Equal(t, diag.StatePresent, row[0].State)
	assert.Equal(t, "headless", row[0].Value.Str)

	rendered := snapshot.JSON()
	assert.NotContains(t, rendered, `"source"`, "an overview names no sources")
	assert.NotContains(t, rendered, "test", "and carries no ref to name them by")

	explained, err := registry.Explain(t.Context(), diag.CategoryRuntime, "mode")
	require.NoError(t, err)
	assert.Equal(t, "test", explained.Field.Effective.Source.Ref,
		"the reader who wants the source asks for the field")
}

// The cheaper row is only worth having if it is genuinely cheaper, and the
// budget has to agree: a row charged for provenance it does not carry would
// starve the catalog for bytes nobody was ever sent.
func TestTheCheaperRowBuysMoreOfTheCatalog(t *testing.T) {
	withRefs := make([]diag.Collector, 0, len(diag.Categories()))
	for _, category := range diag.Categories() {
		collector := bulky(category, crowdedFields)
		for i := range collector.fields {
			collector.fields[i].Effective.Source.Ref = strings.Repeat("x", 120)
		}
		withRefs = append(withRefs, collector)
	}
	long, err := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), withRefs...).
		Snapshot(t.Context(), "")
	require.NoError(t, err)

	short, err := crowded().Snapshot(t.Context(), "")
	require.NoError(t, err)

	assert.Equal(t, rowsPerCategory(short), rowsPerCategory(long),
		"a ref nobody is shown costs nobody a row")
}
