package diag_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// MaxTotalBytes is a promise about the answer that leaves this module, and
// the only way to keep it is to measure the answer rather than an estimate of
// it. The estimate this replaced was built from string lengths and a constant
// per row, which left out the JSON key names, the braces and the commas of a
// structure nine members deep on three layers, and undercounted by about
// three times — so a cap that was declared as binding bound nothing, and a
// detail view honestly reported that it had truncated nothing at forty-seven
// thousand bytes.
//
// Every shape of answer is checked, because the undercount was in the cost of
// a field and a field is in all of them.
func TestNoAnswerIsLargerThanTheBudgetItWasTakenUnder(t *testing.T) {
	limit := diag.DefaultLimits().MaxTotalBytes
	registry := crowded()

	overview, err := registry.Snapshot(t.Context(), "")
	require.NoError(t, err)
	assert.LessOrEqual(t, len(overview.JSON()), limit, "the overview fits the cap it declares")

	for _, category := range diag.Categories() {
		detail, err := registry.Snapshot(t.Context(), category)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(detail.JSON()), limit, "%s fits the cap it declares", category)

		explained, err := registry.Explain(t.Context(), category, "field-00")
		require.NoError(t, err)
		assert.LessOrEqual(t, len(explained.JSON()), limit, "%s explains inside the cap", category)
	}
}

// listField is a field whose every layer carries the largest list the value
// bounds admit. Three layers of MaxListItems strings of MaxValueBytes is more
// than a whole answer may cost, so this is the one shape a field can take
// that no per-string bound can hold — and explain returns exactly one field,
// with no other row to drop instead.
func listField(key string, items, size int) diag.Field {
	list := make([]string, 0, items)
	for range items {
		list = append(list, strings.Repeat("x", size))
	}
	source := diag.Source{Kind: diag.SourceComputed, Ref: "test"}
	return diag.Field{
		Key:        key,
		Configured: diag.Present(diag.ListValue(list), source),
		Loaded:     diag.Present(diag.ListValue(list), source),
		Effective:  diag.Present(diag.ListValue(list), source),
		Apply:      diag.ApplyImmediate,
		Scope:      diag.ScopeProcess,
	}
}

func TestAFieldTooLargeForAnyAnswerIsCutAndSaysSo(t *testing.T) {
	limits := diag.DefaultLimits()
	registry := diag.NewRegistry(fixedClock(), limits, &fake{
		category: diag.CategoryRuntime,
		status:   available("huge"),
		fields:   []diag.Field{listField("huge", limits.MaxListItems, limits.MaxValueBytes)},
	})

	explained, err := registry.Explain(t.Context(), diag.CategoryRuntime, "huge")
	require.NoError(t, err)
	assert.LessOrEqual(t, len(explained.JSON()), limits.MaxTotalBytes)
	assert.True(t, explained.Truncated, "a field that had to be cut says so")
	assert.Contains(t, explained.Note, "cut to fit size limits")
	assert.NotEmpty(t, explained.Field.Effective.Value.List,
		"cutting a field down is not the same as emptying it")

	detail, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(detail.JSON()), limits.MaxTotalBytes)
	assert.True(t, detail.Truncated)
	assert.Contains(t, detail.Note, "narrow it with category")
}

// A field that fits is left alone, so the flag means something when it is
// set: an explanation that always claimed to be cut would say nothing.
func TestAFieldThatFitsIsNotReportedAsCut(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), &fake{
		category: diag.CategoryRuntime,
		status:   available("mode"),
		fields:   []diag.Field{stringField("mode", "headless")},
	})

	explained, err := registry.Explain(t.Context(), diag.CategoryRuntime, "mode")
	require.NoError(t, err)
	assert.False(t, explained.Truncated)
	assert.Empty(t, explained.Note)
}

// The cap has to bind at whatever size it is set to, not only at its default,
// because callers set their own. A budget honored at 32 KB by accident of
// the fixture would not be honored at 4.
func TestTheCapBindsAtWhateverSizeItIsSetTo(t *testing.T) {
	for _, total := range []int{4096, 8192, 16384, 32768} {
		limits := diag.DefaultLimits()
		limits.MaxTotalBytes = total
		collectors := make([]diag.Collector, 0, len(diag.Categories()))
		for _, category := range diag.Categories() {
			collectors = append(collectors, bulky(category, crowdedFields))
		}
		registry := diag.NewRegistry(fixedClock(), limits, collectors...)

		overview, err := registry.Snapshot(t.Context(), "")
		require.NoError(t, err)
		assert.LessOrEqual(t, len(overview.JSON()), total, "overview at total_bytes=%d", total)

		detail, err := registry.Snapshot(t.Context(), diag.CategoryPlan)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(detail.JSON()), total, "detail at total_bytes=%d", total)
	}
}

// The budget is only worth measuring precisely if the precision is spent on
// content: an answer that stopped at half the cap would be dropping fields
// nobody needed to lose. The overview is the crowded case — every category at
// once — so it is where the fit is tightest.
func TestABudgetedAnswerFillsTheBudgetItWasGiven(t *testing.T) {
	overview, err := crowded().Snapshot(t.Context(), "")
	require.NoError(t, err)

	size := len(overview.JSON())
	limit := diag.DefaultLimits().MaxTotalBytes
	require.True(t, overview.Truncated, "the fixture has more to say than the cap holds")
	assert.Greater(t, size, limit*9/10,
		"an answer that was cut short should have been cut close to the cap, not far below it")
}
