package diag_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// slow is a collector whose owner is busy: it does not answer until the
// context it was handed says to stop. It is what a real owner holding its
// own mutex looks like from here, and it is the only kind of stuck owner
// the registry can do anything about — a collector that ignores its context
// is a collector that has stopped being a read, which is the Collector
// contract's problem and not the registry's.
type slow struct {
	category diag.Category
	entered  chan struct{}
	returned chan struct{}
}

func newSlow(category diag.Category) *slow {
	return &slow{
		category: category,
		entered:  make(chan struct{}, 8),
		returned: make(chan struct{}, 8),
	}
}

func (s *slow) Category() diag.Category { return s.category }

func (*slow) Status() diag.Status { return available("busy") }

func (s *slow) Collect(ctx context.Context) ([]diag.Field, error) {
	signal(s.entered)
	<-ctx.Done()
	signal(s.returned)
	return nil, ctx.Err()
}

// signal records that the collector reached a point without ever waiting
// for somebody to be listening, so the test that watches these signals and
// the test that only counts goroutines can share one collector.
func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// budgets is the limits with the time bounds a test names and every size
// bound left at its default, so a time test never trips a size one.
func budgets(category, total time.Duration) diag.Limits {
	limits := diag.DefaultLimits()
	limits.MaxCategoryDuration = category
	limits.MaxTotalDuration = total
	return limits
}

// entryFor is one category's place in an answer.
func entryFor(t *testing.T, snapshot diag.Snapshot, category diag.Category) diag.CategorySnapshot {
	t.Helper()
	for _, entry := range snapshot.Categories {
		if entry.Category == category {
			return entry
		}
	}
	t.Fatalf("the answer has no %s category", category)
	return diag.CategorySnapshot{}
}

// A busy owner costs its own category and nothing else. The alternative —
// one stuck owner failing the whole snapshot — would make the harness
// useless exactly when something is wrong, which is when it is asked.
func TestABusyOwnerCostsItsOwnCategoryAndNoOther(t *testing.T) {
	busy := newSlow(diag.CategoryPlan)
	quick := &fake{category: diag.CategoryRuntime, status: available("mode")}
	quick.fields = []diag.Field{stringField("mode", "tui")}
	registry := diag.NewRegistry(fixedClock(), budgets(20*time.Millisecond, 5*time.Second), busy, quick)

	snapshot, err := registry.Snapshot(t.Context(), "")
	require.NoError(t, err, "a stuck owner is a hole in the answer, not a failure of it")

	stuck := entryFor(t, snapshot, diag.CategoryPlan)
	assert.Equal(t, diag.AvailabilityUnavailable, stuck.Availability)
	assert.Contains(t, stuck.Reason, "busy rather than broken")
	assert.Empty(t, stuck.Overview, "nothing was read, so nothing is reported")

	answered := entryFor(t, snapshot, diag.CategoryRuntime)
	assert.Equal(t, diag.AvailabilityAvailable, answered.Availability)
	assert.True(t, snapshot.Partial, "an answer with a hole in it says so")
	assert.Contains(t, snapshot.Note, "could not be observed")

	// The wait really ended: the collector returned rather than being left
	// behind a snapshot that had already been handed back.
	select {
	case <-busy.returned:
	case <-time.After(2 * time.Second):
		t.Fatal("the category budget must end the collector's wait, not abandon it")
	}
}

// The whole answer is bounded too, because the overview reads every category
// in turn: without it, eleven stuck owners would cost eleven times the
// per-category budget and the turn would be held open for all of it.
func TestAnAnswerThatRunsOutOfTimeStillListsEveryCategory(t *testing.T) {
	first := newSlow(diag.CategoryRuntime)
	second := newSlow(diag.CategoryModel)
	registry := diag.NewRegistry(fixedClock(), budgets(time.Second, 30*time.Millisecond), first, second)

	started := time.Now()
	snapshot, err := registry.Snapshot(t.Context(), "")
	require.NoError(t, err)
	assert.Less(t, time.Since(started), time.Second,
		"the answer's budget bounds the whole read, not each category separately")

	require.Len(t, snapshot.Categories, len(diag.Categories()),
		"a category that vanished from an answer would read as a category that does not exist")
	names := make([]diag.Category, 0, len(snapshot.Categories))
	for _, entry := range snapshot.Categories {
		names = append(names, entry.Category)
	}
	assert.Equal(t, diag.Categories(), names, "unreached categories keep their place in the order")

	assert.Contains(t, entryFor(t, snapshot, diag.CategoryRuntime).Reason, "ran out while this category",
		"the first owner was reached and was cut short")
	assert.Contains(t, entryFor(t, snapshot, diag.CategoryModel).Reason, "before this category was reached",
		"the second was never started, which is a different thing to report")
	assert.True(t, snapshot.Partial)

	assert.Empty(t, second.entered, "an owner past the budget is not read at all")
}

// A per-category budget wider than the whole answer's is not a budget: the
// first stuck owner would spend everything and every category after it would
// be blamed for a wait that was the first one's.
func TestThePerCategoryBudgetIsNarrowedToTheAnswersOwn(t *testing.T) {
	limits := budgets(time.Hour, 30*time.Millisecond)
	registry := diag.NewRegistry(fixedClock(), limits, newSlow(diag.CategoryRuntime))

	started := time.Now()
	_, err := registry.Snapshot(t.Context(), "")
	require.NoError(t, err)
	assert.Less(t, time.Since(started), time.Second)

	acting := fieldByKey(t, processFields(t, diag.DiagnosticDeps{Response: limits}), diag.KeyHarnessLimits)
	assert.Contains(t, acting.Effective.Value.List, "category_time=30ms",
		"the view reports the budget that is acting, not the one it was asked for")
	assert.Contains(t, acting.Loaded.Value.List, "category_time=1h0m0s")
}

// A caller that stopped waiting is not a caller that wants a partial answer.
// Nothing went wrong, so the error is theirs and no record calls it a
// failure.
func TestACallerThatStopsWaitingGetsTheirOwnErrorBack(t *testing.T) {
	var events []diag.AuditEvent
	registry := diag.NewRegistry(fixedClock(), budgets(time.Second, 5*time.Second),
		newSlow(diag.CategoryRuntime)).
		WithAudit(func(event diag.AuditEvent) { events = append(events, event) })

	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	snapshot, err := registry.Snapshot(ctx, "")
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, snapshot.Categories, "an abandoned answer is not returned half-built")

	require.Len(t, events, 1)
	assert.Equal(t, diag.AuditCanceled, events[0].Result)
	assert.NotEqual(t, diag.AuditFailed, events[0].Result, "nothing went wrong; the caller left")
}

// Cancellation and a spent budget both end the real work. Neither leaves a
// goroutine behind, because there is none to leave: the registry starts no
// goroutine at all and the deadline travels on the context the collector is
// already watching.
func TestNoSnapshotLeavesAGoroutineBehind(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), budgets(10*time.Millisecond, 40*time.Millisecond),
		newSlow(diag.CategoryRuntime), newSlow(diag.CategoryModel))

	// One answer first, so any lazily created goroutine exists before the
	// count is taken and is not mistaken for a leak.
	_, err := registry.Snapshot(t.Context(), "")
	require.NoError(t, err)
	settle()
	before := runtime.NumGoroutine()

	for range 8 {
		_, err := registry.Snapshot(t.Context(), "")
		require.NoError(t, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = registry.Snapshot(ctx, "")
	require.Error(t, err)

	settle()
	assert.LessOrEqual(t, runtime.NumGoroutine(), before+1,
		"repeated snapshots must not accumulate goroutines")
}

// settle gives goroutines that are on their way out a chance to finish, so a
// count taken right after a wait ends is not a race with the scheduler.
func settle() {
	for range 50 {
		runtime.Gosched()
		time.Sleep(2 * time.Millisecond)
	}
}

// Explain waits under the same budget, and says which budget it was. A
// deadline is not an explanation: "context deadline exceeded" tells a reader
// nothing about what to do next.
func TestExplainIsBoundedByTheSameBudget(t *testing.T) {
	var events []diag.AuditEvent
	registry := diag.NewRegistry(fixedClock(), budgets(20*time.Millisecond, 5*time.Second),
		newSlow(diag.CategoryRuntime)).
		WithAudit(func(event diag.AuditEvent) { events = append(events, event) })

	_, err := registry.Explain(t.Context(), diag.CategoryRuntime, "busy")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "busy rather than broken")
	assert.NotContains(t, err.Error(), "context deadline exceeded")

	require.Len(t, events, 1)
	assert.Equal(t, diag.AuditFailed, events[0].Result)
	assert.Equal(t, "busy", events[0].Key)
}
