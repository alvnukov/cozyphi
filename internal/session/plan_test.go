package session

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func TestUpdatePlanDoesNotMoveConversationLeafOrContext(t *testing.T) {
	m := NewManager(t.TempDir())
	_, err := m.Append(llm.Message{Role: llm.RoleUser, Content: "keep me"})
	require.NoError(t, err)
	leaf := m.LeafID()
	before := messageContents(m.BuildContext())

	plan, err := m.UpdatePlan(0, []PlanItem{{Content: " inspect ", Status: PlanInProgress}})
	require.NoError(t, err)

	assert.Equal(t, leaf, m.LeafID())
	assert.Equal(t, before, messageContents(m.BuildContext()))
	assert.Equal(t, "inspect", plan.Items[0].Content)
	assert.Equal(t, plan, m.Plan())
}

func TestPlanPersistsAndRestoresLatestSnapshot(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)

	first, err := m.UpdatePlan(0, []PlanItem{{
		Content: "first",
		Status:  PlanBlocked,
		Note:    " waiting for approval ",
	}})
	require.NoError(t, err)
	latest, err := m.UpdatePlan(first.Revision, []PlanItem{
		{Content: "first", Status: PlanCompleted, Evidence: " targeted test passed "},
		{Content: "second", Status: PlanPending},
	})
	require.NoError(t, err)
	require.Equal(t, first.Revision+1, latest.Revision)

	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	restored := loaded.Plan()
	assert.Equal(t, latest.Revision, restored.Revision)
	assert.Equal(t, latest.Items, restored.Items)
	assert.Equal(t, "targeted test passed", restored.Items[0].Evidence)
	assert.True(t, latest.UpdatedAt.Equal(restored.UpdatedAt))
	assert.Empty(t, loaded.LeafID(), "plan metadata must not become a conversation leaf")
	assert.Empty(t, loaded.BuildContext(), "plan metadata must stay out of provider context")
}

func TestOpenSessionAcceptsPlansWrittenUnderPreviousLimits(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)

	items := make([]PlanItem, 33)
	for i := range items {
		items[i] = PlanItem{Content: strings.Repeat("x", 300), Status: PlanPending}
	}
	now := time.Now()
	entry := PlanEntry{
		SessionBaseEntry: SessionBaseEntry{Type: EntryPlan, ID: m.generateID(), Timestamp: now},
		Plan:             Plan{Revision: 1, UpdatedAt: now, Items: items},
	}
	m.entries = append(m.entries, entry)
	m.byIDs[entry.ID] = entry
	require.NoError(t, m.flush(entry))

	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	restored := loaded.Plan()
	assert.Equal(t, entry.Plan.Revision, restored.Revision)
	assert.Equal(t, entry.Plan.Items, restored.Items)
	assert.True(t, entry.Plan.UpdatedAt.Equal(restored.UpdatedAt))
}

func TestUpdatePlanRejectsStaleRevisionWithoutMutation(t *testing.T) {
	m := NewManager(t.TempDir())
	current, err := m.UpdatePlan(0, []PlanItem{{Content: "current", Status: PlanInProgress}})
	require.NoError(t, err)

	_, err = m.UpdatePlan(0, []PlanItem{{Content: "stale overwrite", Status: PlanCompleted}})
	require.ErrorIs(t, err, ErrPlanRevisionConflict)
	assert.Contains(t, err.Error(), "expected 0, current 1")
	assert.Equal(t, current, m.Plan())
}

func TestUpdatePlanRejectsInvalidSnapshotsWithoutMutation(t *testing.T) {
	m := NewManager(t.TempDir())
	valid, err := m.UpdatePlan(0, []PlanItem{{Content: "one", Status: PlanPending}})
	require.NoError(t, err)

	large := make([]PlanItem, maxPlanItems)
	for i := range large {
		large[i] = PlanItem{
			Content:  strings.Repeat("c", maxPlanContentRunes),
			Status:   PlanBlocked,
			Note:     strings.Repeat("n", maxPlanNoteRunes),
			Evidence: strings.Repeat("e", maxPlanEvidenceRunes),
		}
	}
	cases := map[string][]PlanItem{
		"empty content":    {{Content: "  ", Status: PlanPending}},
		"invalid status":   {{Content: "one", Status: "started"}},
		"too long content": {{Content: strings.Repeat("x", maxPlanContentRunes+1), Status: PlanPending}},
		"too long note": {{
			Content: "one",
			Status:  PlanBlocked,
			Note:    strings.Repeat("x", maxPlanNoteRunes+1),
		}},
		"too long evidence": {{
			Content:  "one",
			Status:   PlanCompleted,
			Evidence: strings.Repeat("x", maxPlanEvidenceRunes+1),
		}},
		"too many items": append(
			large,
			PlanItem{Content: "overflow", Status: PlanPending},
		),
		"too many bytes": large,
		"two active": {
			{Content: "one", Status: PlanInProgress},
			{Content: "two", Status: PlanInProgress},
		},
	}
	for name, items := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := m.UpdatePlan(valid.Revision, items)
			require.Error(t, err)
			assert.Equal(t, valid, m.Plan())
		})
	}
}

func TestUpdatePlanAllowsBlockedAlongsideOneActiveStep(t *testing.T) {
	m := NewManager(t.TempDir())
	plan, err := m.UpdatePlan(0, []PlanItem{
		{Content: "external one", Status: PlanBlocked, Note: "needs approval"},
		{Content: "external two", Status: PlanBlocked},
		{Content: "local", Status: PlanInProgress},
	})
	require.NoError(t, err)
	assert.Equal(t, PlanBlocked, plan.Items[0].Status)
	assert.Equal(t, PlanBlocked, plan.Items[1].Status)
}

func TestUpdatePlanCompareAndSwapSerializesConcurrentSnapshots(t *testing.T) {
	m := NewManager(t.TempDir())
	const updates = 24
	errCh := make(chan error, updates)
	var wg sync.WaitGroup
	for range updates {
		wg.Go(func() {
			_, err := m.UpdatePlan(0, []PlanItem{{Content: "step", Status: PlanCompleted}})
			errCh <- err
		})
	}
	wg.Wait()
	close(errCh)

	successes := 0
	conflicts := 0
	for err := range errCh {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrPlanRevisionConflict):
			conflicts++
		default:
			require.NoError(t, err)
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, updates-1, conflicts)
	assert.Equal(t, uint64(1), m.Plan().Revision)
}

func TestUpdatePlanPersistenceFailureDoesNotPublishState(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	m.sessionFile = dir // a directory cannot be atomically used as the JSONL file

	_, err = m.UpdatePlan(0, []PlanItem{{Content: "must fail", Status: PlanPending}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "persist plan")
	assert.Zero(t, m.Plan().Revision)
	assert.Empty(t, m.Plan().Items)
}

func TestUpdatePlanRejectsInvalidStepType(t *testing.T) {
	m := NewManager(t.TempDir())
	valid, err := m.UpdatePlan(0, []PlanItem{{Content: "one", Status: PlanPending}})
	require.NoError(t, err)

	_, err = m.UpdatePlan(valid.Revision, []PlanItem{
		{Content: "bad type", Status: PlanPending, Type: "vibe"},
	})
	require.Error(t, err)
	assert.Equal(t, valid, m.Plan(), "invalid step type must not mutate the plan")
}

func TestUpdatePlanAcceptsKnownStepTypes(t *testing.T) {
	m := NewManager(t.TempDir())
	plan, err := m.UpdatePlan(0, []PlanItem{
		{Content: "look", Status: PlanPending, Type: StepExplore},
		{Content: "write", Status: PlanPending, Type: StepEdit},
		{Content: "run", Status: PlanPending, Type: StepRun},
		{Content: "child", Status: PlanPending, Type: StepDelegate},
		{Content: "mcp", Status: PlanPending, Type: StepIntegrate},
	})
	require.NoError(t, err)
	require.Len(t, plan.Items, 5)
	assert.Equal(t, StepExplore, plan.Items[0].Type)
	assert.Equal(t, StepEdit, plan.Items[1].Type)
	assert.Equal(t, StepRun, plan.Items[2].Type)
	assert.Equal(t, StepDelegate, plan.Items[3].Type)
	assert.Equal(t, StepIntegrate, plan.Items[4].Type)
}

func TestApprovePlanBumpsRevisionWithoutTouchingItems(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)

	_, err = m.UpdatePlan(0, []PlanItem{
		{Content: "step", Status: PlanInProgress, Type: StepEdit},
	})
	require.NoError(t, err)

	approved, err := m.SetPlanApproved(true)
	require.NoError(t, err)
	assert.True(t, approved.Approved)
	assert.Equal(t, uint64(2), approved.Revision)
	assert.Equal(t, []PlanItem{{Content: "step", Status: PlanInProgress, Type: StepEdit}}, approved.Items)

	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	restored := loaded.Plan()
	assert.True(t, restored.Approved, "approval must survive resume")
	assert.Equal(t, approved.Revision, restored.Revision)
	assert.Equal(t, approved.Items, restored.Items)
	assert.True(t, approved.UpdatedAt.Equal(restored.UpdatedAt))
}

func TestUpdatePlanUnapprovesOnStepUpdate(t *testing.T) {
	m := NewManager(t.TempDir())
	plan, err := m.UpdatePlan(0, []PlanItem{{Content: "step", Status: PlanInProgress, Type: StepEdit}})
	require.NoError(t, err)
	require.False(t, plan.Approved)

	approved, err := m.SetPlanApproved(true)
	require.NoError(t, err)
	require.True(t, approved.Approved)

	updated, err := m.UpdatePlan(
		approved.Revision,
		[]PlanItem{{Content: "step 2", Status: PlanInProgress, Type: StepEdit}},
	)
	require.NoError(t, err)
	assert.False(t, updated.Approved, "any step update must drop approval")
}
