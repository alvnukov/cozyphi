package diag_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// fake is a collector under the test's control: it stands in for the fifteen
// collectors still to be written, so the guarantees the registry makes for
// all of them (sanitization, bounds, detachment, error containment) are
// asserted against a collector that deliberately misbehaves.
type fake struct {
	category  diag.Category
	status    diag.Status
	fields    []diag.Field
	err       error
	collected int
}

func (f *fake) Category() diag.Category { return f.category }

func (f *fake) Status() diag.Status { return f.status }

func (f *fake) Collect(context.Context) ([]diag.Field, error) {
	f.collected++
	if f.err != nil {
		return nil, f.err
	}
	return f.fields, nil
}

func available(keys ...string) diag.Status {
	return diag.Status{Availability: diag.AvailabilityAvailable, Keys: keys}
}

func stringField(key, value string) diag.Field {
	source := diag.Source{Kind: diag.SourceComputed, Ref: "test"}
	return diag.Field{
		Key:        key,
		Configured: diag.NotApplicable(diag.SourceComputed),
		Loaded:     diag.NotApplicable(diag.SourceComputed),
		Effective:  diag.Present(diag.StringValue(value), source),
		Apply:      diag.ApplyImmediate,
		Scope:      diag.ScopeProcess,
	}
}

func fixedClock() func() time.Time {
	at := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return at }
}

func TestCatalogListsEveryCategoryAndIsHonestAboutGaps(t *testing.T) {
	runtime := &fake{
		category: diag.CategoryRuntime,
		status:   available("version", "mode"),
	}
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), runtime)

	catalog := registry.Catalog()
	require.Len(t, catalog.Categories, len(diag.Categories()))

	names := make([]diag.Category, 0, len(catalog.Categories))
	for _, entry := range catalog.Categories {
		names = append(names, entry.Category)
	}
	assert.Equal(t, diag.Categories(), names, "the catalog follows one fixed order")

	assert.Equal(t, diag.AvailabilityAvailable, catalog.Categories[0].Availability)
	assert.Equal(t, []string{"version", "mode"}, catalog.Categories[0].Keys)

	for _, entry := range catalog.Categories[1:] {
		assert.Equal(t, diag.AvailabilityNotImplemented, entry.Availability, string(entry.Category))
		assert.NotEmpty(t, entry.Reason, "an unimplemented category says so instead of looking empty")
		assert.Empty(t, entry.Keys, string(entry.Category))
	}
	assert.Zero(t, runtime.collected, "listing what can be observed must not observe anything")
}

func TestCatalogKeysAreDetachedFromTheCollector(t *testing.T) {
	keys := []string{"version"}
	registry := diag.NewRegistry(nil, diag.DefaultLimits(), &fake{
		category: diag.CategoryRuntime,
		status:   available(keys...),
	})

	catalog := registry.Catalog()
	catalog.Categories[0].Keys[0] = "tampered"

	assert.Equal(t, "version", registry.Catalog().Categories[0].Keys[0])
	assert.Equal(t, "version", keys[0], "the collector's own slice must not be reachable")
}

func TestSnapshotOverviewCoversEveryCategoryWithEffectiveValuesOnly(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), &fake{
		category: diag.CategoryRuntime,
		status:   available("mode"),
		fields:   []diag.Field{stringField("mode", "headless")},
	})

	snapshot, err := registry.Snapshot(t.Context(), "")
	require.NoError(t, err)
	assert.Equal(t, diag.ModeOverview, snapshot.Mode)
	require.Len(t, snapshot.Categories, len(diag.Categories()))

	runtime := snapshot.Categories[0]
	require.Len(t, runtime.Overview, 1)
	assert.Equal(t, "mode", runtime.Overview[0].Key)
	assert.Equal(t, "headless", runtime.Overview[0].Value.Str)
	assert.Empty(t, runtime.Fields, "an overview carries no layers")
	assert.False(t, snapshot.Truncated)
	assert.False(t, snapshot.Partial)
	assert.Empty(t, snapshot.Note)
}

func TestSnapshotDetailCarriesEveryLayerAndProvenance(t *testing.T) {
	at := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), &fake{
		category: diag.CategoryRuntime,
		status:   available("mode"),
		fields:   []diag.Field{stringField("mode", "headless")},
	})

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	assert.Equal(t, diag.ModeDetail, snapshot.Mode)
	require.Len(t, snapshot.Categories, 1)

	runtime := snapshot.Categories[0]
	assert.Empty(t, runtime.Overview, "a detail view carries fields, not the overview rows")
	require.Len(t, runtime.Fields, 1)
	field := runtime.Fields[0]
	assert.Equal(t, diag.StateNotApplicable, field.Configured.State)
	assert.Equal(t, diag.StateNotApplicable, field.Loaded.State)
	assert.Equal(t, diag.StatePresent, field.Effective.State)
	assert.Equal(t, diag.SourceComputed, field.Effective.Source.Kind)
	assert.Equal(t, diag.ApplyImmediate, field.Apply)
	assert.Equal(t, diag.ScopeProcess, field.Scope)
	assert.Equal(t, at, field.ObservedAt, "the registry stamps the clock, not the collector")
	assert.Equal(t, at, runtime.ObservedAt)
}

func TestSnapshotRejectsAnUnknownCategory(t *testing.T) {
	registry := diag.NewRegistry(nil, diag.DefaultLimits())

	_, err := registry.Snapshot(t.Context(), diag.Category("wallet"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown category")
	assert.Contains(t, err.Error(), "runtime", "the error names what is known")
}

func TestSnapshotOfAnUnwiredCategoryReportsTheGap(t *testing.T) {
	registry := diag.NewRegistry(nil, diag.DefaultLimits())

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryStorage)
	require.NoError(t, err, "an unimplemented category is information, not a failure")
	require.Len(t, snapshot.Categories, 1)
	assert.Equal(t, diag.AvailabilityNotImplemented, snapshot.Categories[0].Availability)
	assert.Empty(t, snapshot.Categories[0].Fields)
}

func TestCollectorErrorCostsOneCategoryAndMarksPartial(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		&fake{
			category: diag.CategoryRuntime,
			status:   available("mode"),
			err:      errors.New("owner is busy holding ghp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		},
		&fake{
			category: diag.CategoryPlan,
			status:   available("mode"),
			fields:   []diag.Field{stringField("mode", "useplan")},
		},
	)

	snapshot, err := registry.Snapshot(t.Context(), "")
	require.NoError(t, err, "one broken owner must not fail the whole snapshot")
	assert.True(t, snapshot.Partial)
	assert.Contains(t, snapshot.Note, "could not be observed")

	assert.Equal(t, diag.AvailabilityUnavailable, snapshot.Categories[0].Availability)
	assert.Contains(t, snapshot.Categories[0].Reason, "owner is busy")
	assert.NotContains(t, snapshot.Categories[0].Reason, "ghp_", "error text is sanitized too")

	plan := snapshot.Categories[4]
	require.Equal(t, diag.CategoryPlan, plan.Category)
	assert.Equal(t, diag.AvailabilityAvailable, plan.Availability)
	assert.Len(t, plan.Overview, 1, "the categories that worked still answer")
}

func TestStatusUnavailableSkipsCollectionEntirely(t *testing.T) {
	collector := &fake{
		category: diag.CategoryRuntime,
		status: diag.Status{
			Availability: diag.AvailabilityUnavailable,
			Reason:       "the owner is not built yet",
			Keys:         []string{"mode"},
		},
		fields: []diag.Field{stringField("mode", "headless")},
	}
	registry := diag.NewRegistry(nil, diag.DefaultLimits(), collector)

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	assert.Equal(t, diag.AvailabilityUnavailable, snapshot.Categories[0].Availability)
	assert.Equal(t, "the owner is not built yet", snapshot.Categories[0].Reason)
	assert.Empty(t, snapshot.Categories[0].Fields)
	assert.Zero(t, collector.collected)
}

func TestSnapshotMasksSecretsAndReportsThemAsRedacted(t *testing.T) {
	const sentinel = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	registry := diag.NewRegistry(nil, diag.DefaultLimits(), &fake{
		category: diag.CategoryRuntime,
		status:   available("token", "tokens"),
		fields: []diag.Field{
			stringField("token", "authorization: "+sentinel),
			{
				Key:        "tokens",
				Configured: diag.NotApplicable(diag.SourceComputed),
				Loaded:     diag.NotApplicable(diag.SourceComputed),
				Effective: diag.Present(
					diag.ListValue([]string{"safe", sentinel}),
					diag.Source{Kind: diag.SourceComputed, Ref: sentinel},
				),
			},
		},
	})

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	rendered := snapshot.JSON()
	assert.NotContains(t, rendered, sentinel, "a collector must not be able to leak by omission")
	assert.Contains(t, rendered, "[REDACTED]")

	require.Len(t, snapshot.Categories[0].Fields, 2)
	assert.Equal(t, diag.StateRedacted, snapshot.Categories[0].Fields[0].Effective.State)
	assert.Equal(t, diag.StateRedacted, snapshot.Categories[0].Fields[1].Effective.State)
	assert.Equal(t, "safe", snapshot.Categories[0].Fields[1].Effective.Value.List[0])
}

func TestSnapshotCollapsesTheHomeDirectoryAndStripsControlCharacters(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	registry := diag.NewRegistry(nil, diag.DefaultLimits(), &fake{
		category: diag.CategoryRuntime,
		status:   available("root"),
		fields:   []diag.Field{stringField("root", home+"/src/cozyphi\x1b[31m\n")},
	})

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	value := snapshot.Categories[0].Fields[0].Effective.Value.Str
	assert.Equal(t, "~/src/cozyphi[31m", value)
	assert.NotContains(t, value, home)
	assert.NotContains(t, value, "\x1b")
}

func TestSnapshotTruncatesByFieldCountAndSaysHowToNarrow(t *testing.T) {
	fields := make([]diag.Field, 0, 4)
	keys := make([]string, 0, 4)
	for _, key := range []string{"a", "b", "c", "d"} {
		fields = append(fields, stringField(key, key))
		keys = append(keys, key)
	}
	limits := diag.DefaultLimits()
	limits.MaxFieldsPerCategory = 2
	registry := diag.NewRegistry(nil, limits, &fake{
		category: diag.CategoryRuntime,
		status:   available(keys...),
		fields:   fields,
	})

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	assert.True(t, snapshot.Truncated)
	assert.True(t, snapshot.Categories[0].Truncated)
	assert.Len(t, snapshot.Categories[0].Fields, 2)
	assert.Contains(t, snapshot.Note, "narrow it with category")
}

func TestSnapshotTruncatesByTotalBytes(t *testing.T) {
	limits := diag.DefaultLimits()
	limits.MaxTotalBytes = 1
	registry := diag.NewRegistry(nil, limits, &fake{
		category: diag.CategoryRuntime,
		status:   available("mode"),
		fields:   []diag.Field{stringField("mode", "headless")},
	})

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	assert.True(t, snapshot.Truncated)
	assert.Empty(t, snapshot.Categories[0].Fields)
}

func TestSnapshotBoundsValueBytesAndListItems(t *testing.T) {
	limits := diag.DefaultLimits()
	limits.MaxValueBytes = 16
	limits.MaxListItems = 2
	registry := diag.NewRegistry(nil, limits, &fake{
		category: diag.CategoryRuntime,
		status:   available("long", "many"),
		fields: []diag.Field{
			stringField("long", strings.Repeat("x", 200)),
			{
				Key:        "many",
				Configured: diag.NotApplicable(diag.SourceComputed),
				Loaded:     diag.NotApplicable(diag.SourceComputed),
				Effective: diag.Present(
					diag.ListValue([]string{"one", "two", "three", "four"}),
					diag.Source{Kind: diag.SourceComputed},
				),
			},
		},
	})

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	long := snapshot.Categories[0].Fields[0].Effective.Value.Str
	assert.LessOrEqual(t, len(long), 16)
	assert.True(t, strings.HasSuffix(long, "…"), "a cut value never reads as a complete one")
	assert.Len(t, snapshot.Categories[0].Fields[1].Effective.Value.List, 2)
	assert.True(t, snapshot.Truncated)
}

func TestSnapshotResultIsDetachedFromTheCollector(t *testing.T) {
	original := []string{"one", "two"}
	collector := &fake{
		category: diag.CategoryRuntime,
		status:   available("many"),
		fields: []diag.Field{{
			Key:        "many",
			Configured: diag.NotApplicable(diag.SourceComputed),
			Loaded:     diag.NotApplicable(diag.SourceComputed),
			Effective: diag.Present(
				diag.Value{Kind: diag.KindList, List: original},
				diag.Source{Kind: diag.SourceComputed},
			),
		}},
	}
	registry := diag.NewRegistry(nil, diag.DefaultLimits(), collector)

	first, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	first.Categories[0].Fields[0].Effective.Value.List[0] = "tampered"

	second, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	assert.Equal(t, "one", second.Categories[0].Fields[0].Effective.Value.List[0])
	assert.Equal(t, "one", original[0], "the owner's slice is never handed out")
}

func TestExplainAnswersOneFieldInFull(t *testing.T) {
	at := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), &fake{
		category: diag.CategoryRuntime,
		status:   available("mode", "other"),
		fields:   []diag.Field{stringField("mode", "headless"), stringField("other", "x")},
	})

	explanation, err := registry.Explain(t.Context(), diag.CategoryRuntime, " mode ")
	require.NoError(t, err)
	assert.Equal(t, diag.CategoryRuntime, explanation.Category)
	assert.Equal(t, diag.AvailabilityAvailable, explanation.Availability)
	assert.Equal(t, "mode", explanation.Field.Key)
	assert.Equal(t, "headless", explanation.Field.Effective.Value.Str)
	assert.Equal(t, at, explanation.ObservedAt)
}

func TestExplainRejectsUnknownCategoryAndKeyBeforeCollecting(t *testing.T) {
	collector := &fake{
		category: diag.CategoryRuntime,
		status:   available("mode"),
		fields:   []diag.Field{stringField("mode", "headless")},
	}
	registry := diag.NewRegistry(nil, diag.DefaultLimits(), collector)

	_, err := registry.Explain(t.Context(), diag.Category("wallet"), "mode")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown category")

	_, err = registry.Explain(t.Context(), diag.CategoryRuntime, "made_up")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no field \"made_up\"")
	assert.Contains(t, err.Error(), "mode", "the error names the keys that do exist")

	_, err = registry.Explain(t.Context(), diag.CategoryStorage, "mode")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")

	assert.Zero(t, collector.collected, "a rejected key costs no observation")
}

func TestFalseAndZeroSurviveSerialization(t *testing.T) {
	registry := diag.NewRegistry(nil, diag.DefaultLimits(), &fake{
		category: diag.CategoryRuntime,
		status:   available("enabled", "count"),
		fields: []diag.Field{
			{
				Key:        "enabled",
				Configured: diag.Unset(diag.BoolValue(false), diag.Source{Kind: diag.SourceDefault}),
				Loaded:     diag.Present(diag.BoolValue(false), diag.Source{Kind: diag.SourceDefault}),
				Effective:  diag.Present(diag.BoolValue(false), diag.Source{Kind: diag.SourceDefault}),
			},
			{
				Key:        "count",
				Configured: diag.Unset(diag.IntValue(0), diag.Source{Kind: diag.SourceDefault}),
				Loaded:     diag.Present(diag.IntValue(0), diag.Source{Kind: diag.SourceDefault}),
				Effective:  diag.Present(diag.IntValue(0), diag.Source{Kind: diag.SourceDefault}),
			},
		},
	})

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	rendered := snapshot.JSON()
	assert.Contains(t, rendered, `"bool": false`)
	assert.Contains(t, rendered, `"int": 0`)
	assert.Contains(t, rendered, `"string": ""`)
	assert.Contains(t, rendered, `"list": []`)
	assert.Contains(t, rendered, `"truncated": false`)
	assert.Contains(t, rendered, `"partial": false`)
}

func TestRenderingIsDeterministic(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), &fake{
		category: diag.CategoryRuntime,
		status:   available("mode"),
		fields:   []diag.Field{stringField("mode", "headless")},
	})

	first, err := registry.Snapshot(t.Context(), "")
	require.NoError(t, err)
	second, err := registry.Snapshot(t.Context(), "")
	require.NoError(t, err)
	assert.Equal(t, first.JSON(), second.JSON())

	catalog := registry.Catalog().JSON()
	assert.Equal(t, catalog, registry.Catalog().JSON())
}

func TestDuplicateCategoryRegistrationIsLastWins(t *testing.T) {
	registry := diag.NewRegistry(nil, diag.DefaultLimits(),
		&fake{category: diag.CategoryRuntime, status: available("first")},
		&fake{category: diag.CategoryRuntime, status: available("second")},
	)

	assert.Equal(t, []string{"second"}, registry.Catalog().Categories[0].Keys)
}

func TestUnknownCategoryCollectorIsDropped(t *testing.T) {
	registry := diag.NewRegistry(nil, diag.DefaultLimits(),
		&fake{category: diag.Category("wallet"), status: available("balance")},
		nil,
	)

	for _, entry := range registry.Catalog().Categories {
		assert.Equal(t, diag.AvailabilityNotImplemented, entry.Availability, string(entry.Category))
	}
}

func TestZeroLimitsFallBackToTheDefaults(t *testing.T) {
	registry := diag.NewRegistry(nil, diag.Limits{}, &fake{
		category: diag.CategoryRuntime,
		status:   available("mode"),
		fields:   []diag.Field{stringField("mode", "headless")},
	})

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	assert.False(t, snapshot.Truncated, "a partly-filled Limits must be bounded, not empty")
	assert.Len(t, snapshot.Categories[0].Fields, 1)
}

func TestNilClockFallsBackToWallTime(t *testing.T) {
	registry := diag.NewRegistry(nil, diag.DefaultLimits(), &fake{
		category: diag.CategoryRuntime,
		status:   available("mode"),
		fields:   []diag.Field{stringField("mode", "headless")},
	})

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), snapshot.Categories[0].ObservedAt, time.Minute)
}

func TestCancelledContextStopsBeforeObserving(t *testing.T) {
	collector := &fake{
		category: diag.CategoryRuntime,
		status:   available("mode"),
		fields:   []diag.Field{stringField("mode", "headless")},
	}
	registry := diag.NewRegistry(nil, diag.DefaultLimits(), collector)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := registry.Snapshot(ctx, diag.CategoryRuntime)
	require.ErrorIs(t, err, context.Canceled)
	_, err = registry.Explain(ctx, diag.CategoryRuntime, "mode")
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, collector.collected)
}

func TestParseCategoryDoesNotGuess(t *testing.T) {
	category, ok := diag.ParseCategory("runtime")
	require.True(t, ok)
	assert.Equal(t, diag.CategoryRuntime, category)

	for _, name := range []string{"", "Runtime", " runtime", "run", "wallet"} {
		_, ok := diag.ParseCategory(name)
		assert.False(t, ok, name)
	}
}

func TestCategoriesIsDetached(t *testing.T) {
	categories := diag.Categories()
	categories[0] = diag.Category("tampered")
	assert.Equal(t, diag.CategoryRuntime, diag.Categories()[0])
}
