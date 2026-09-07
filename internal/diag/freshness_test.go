package diag_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// steppingClock moves one second every time it is read, so an answer that
// reads several owners in turn carries several different moments — which is
// what a snapshot spanning owners really is.
func steppingClock() func() time.Time {
	at := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	return func() time.Time {
		at = at.Add(time.Second)
		return at
	}
}

// A collector that never thought about freshness has, by the Collector
// contract, read its owner during this observation. Saying so once at the
// boundary is what keeps the field from being empty on every collector that
// never thought about it.
func TestAFieldNobodyDatedIsTheAnswerAsOfNow(t *testing.T) {
	runtime := &fake{
		category: diag.CategoryRuntime,
		status:   available("version"),
		fields:   []diag.Field{stringField("version", "1.2.3")},
	}
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), runtime)

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories[0].Fields, 1)
	assert.Equal(t, diag.FreshnessLive, snapshot.Categories[0].Fields[0].Freshness)

	explanation, err := registry.Explain(t.Context(), diag.CategoryRuntime, "version")
	require.NoError(t, err)
	assert.Equal(t, diag.FreshnessLive, explanation.Field.Freshness,
		"one field explained is the same field the snapshot carried")
}

// A collector that did think about it keeps its own answer: the boundary
// fills a gap and never overrules an owner.
func TestAnOwnerThatDatedItsAnswerKeepsIt(t *testing.T) {
	stale := stringField("version", "1.2.3")
	stale.Freshness = diag.FreshnessUnknown
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), &fake{
		category: diag.CategoryRuntime,
		status:   available("version"),
		fields:   []diag.Field{stale},
	})

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	assert.Equal(t, diag.FreshnessUnknown, snapshot.Categories[0].Fields[0].Freshness)
}

// The surface belongs to the render goroutine and hands its account over
// when it changes. Reading that account is what keeps this view off state
// another goroutine owns, and the price is an answer as old as the last
// change rather than as old as the question.
func TestTheSurfaceIsPublishedRatherThanLive(t *testing.T) {
	fields := uiFields(t, liveUIConfig(), liveSurface())
	require.NotEmpty(t, fields)
	for _, field := range fields {
		assert.Equal(t, diag.FreshnessPublished, field.Freshness, field.Key)
	}
}

// A snapshot spanning several owners is not one instant. It is a sequence of
// them, and the timestamps are what say so — a reader comparing two
// categories is comparing two different moments.
func TestASnapshotIsASequenceOfInstantsAndNotOne(t *testing.T) {
	registry := diag.NewRegistry(steppingClock(), diag.DefaultLimits(),
		&fake{
			category: diag.CategoryRuntime,
			status:   available("version"),
			fields:   []diag.Field{stringField("version", "1.2.3")},
		},
		&fake{
			category: diag.CategoryModel,
			status:   available("name"),
			fields:   []diag.Field{stringField("name", "haiku")},
		},
	)

	snapshot, err := registry.Snapshot(t.Context(), "")
	require.NoError(t, err)
	first := entryFor(t, snapshot, diag.CategoryRuntime)
	second := entryFor(t, snapshot, diag.CategoryModel)

	assert.False(t, first.ObservedAt.IsZero(), "every category says when it was read")
	assert.True(t, second.ObservedAt.After(first.ObservedAt),
		"the second owner was read after the first, and the answer admits it")
}

// Within one category the fields do agree: the registry owns the clock and
// stamps them all with the moment that category was read, so two fields from
// one owner are never a moment apart for no reason.
func TestOneCategorysFieldsAgreeWithEachOtherAndWithTheirCategory(t *testing.T) {
	registry := diag.NewRegistry(steppingClock(), diag.DefaultLimits(), &fake{
		category: diag.CategoryRuntime,
		status:   available("version", "mode"),
		fields:   []diag.Field{stringField("version", "1.2.3"), stringField("mode", "interactive")},
	})

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	entry := snapshot.Categories[0]
	require.Len(t, entry.Fields, 2)
	assert.Equal(t, entry.ObservedAt, entry.Fields[0].ObservedAt)
	assert.Equal(t, entry.ObservedAt, entry.Fields[1].ObservedAt)
}
