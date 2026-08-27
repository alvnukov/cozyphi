package session

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func TestReplacePlanDoesNotMoveConversationLeafOrContext(t *testing.T) {
	m := NewManager(t.TempDir())
	_, err := m.Append(llm.Message{Role: llm.RoleUser, Content: "keep me"})
	require.NoError(t, err)
	leaf := m.LeafID()
	before := messageContents(m.BuildContext())

	plan, err := m.ReplacePlan([]PlanItem{{Content: " inspect ", Status: PlanInProgress}})
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

	first, err := m.ReplacePlan([]PlanItem{{
		Content: "first",
		Status:  PlanBlocked,
		Note:    " waiting for approval ",
	}})
	require.NoError(t, err)
	latest, err := m.ReplacePlan([]PlanItem{
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

func TestReplacePlanRejectsInvalidSnapshotsWithoutMutation(t *testing.T) {
	m := NewManager(t.TempDir())
	valid, err := m.ReplacePlan([]PlanItem{{Content: "one", Status: PlanPending}})
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
			_, err := m.ReplacePlan(items)
			require.Error(t, err)
			assert.Equal(t, valid, m.Plan())
		})
	}
}

func TestReplacePlanAllowsBlockedAlongsideOneActiveStep(t *testing.T) {
	m := NewManager(t.TempDir())
	plan, err := m.ReplacePlan([]PlanItem{
		{Content: "external one", Status: PlanBlocked, Note: "needs approval"},
		{Content: "external two", Status: PlanBlocked},
		{Content: "local", Status: PlanInProgress},
	})
	require.NoError(t, err)
	assert.Equal(t, PlanBlocked, plan.Items[0].Status)
	assert.Equal(t, PlanBlocked, plan.Items[1].Status)
}

func TestReplacePlanSerializesConcurrentUpdatesWithoutConflicts(t *testing.T) {
	m := NewManager(t.TempDir())
	const updates = 24
	errCh := make(chan error, updates)
	var wg sync.WaitGroup
	for range updates {
		wg.Go(func() {
			_, err := m.ReplacePlan([]PlanItem{{Content: "step", Status: PlanCompleted}})
			errCh <- err
		})
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
	assert.Equal(t, uint64(updates), m.Plan().Revision)
}

func TestReplacePlanPersistenceFailureDoesNotPublishState(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	m.sessionFile = dir // a directory cannot be atomically used as the JSONL file

	_, err = m.ReplacePlan([]PlanItem{{Content: "must fail", Status: PlanPending}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "persist plan")
	assert.Zero(t, m.Plan().Revision)
	assert.Empty(t, m.Plan().Items)
}

func TestReplacePlanPersistsCustomStepTypeForPolicyValidation(t *testing.T) {
	m := NewManager(t.TempDir())
	plan, err := m.ReplacePlan([]PlanItem{
		{Content: "custom type", Status: PlanPending, Type: "inspect"},
	})
	require.NoError(t, err)
	require.Len(t, plan.Items, 1)
	assert.Equal(t, StepType("inspect"), plan.Items[0].Type)
}

func TestRenamePlanStepTypesPreservesApprovalAndOtherFields(t *testing.T) {
	m := NewManager(t.TempDir())
	before, err := m.ReplacePlanWithAutoApprove([]PlanItem{{
		Content: "inspect", Status: PlanInProgress, Type: "inspect", Note: "keep", Evidence: "also keep",
	}}, true)
	require.NoError(t, err)
	require.True(t, before.Approved)

	after, err := m.RenamePlanStepTypes(map[StepType]StepType{"inspect": "review"})
	require.NoError(t, err)
	assert.Equal(t, before.Revision+1, after.Revision)
	assert.True(t, after.Approved)
	require.Len(t, after.Items, 1)
	assert.Equal(t, StepType("review"), after.Items[0].Type)
	assert.Equal(t, before.Items[0].Content, after.Items[0].Content)
	assert.Equal(t, before.Items[0].Status, after.Items[0].Status)
	assert.Equal(t, before.Items[0].Note, after.Items[0].Note)
	assert.Equal(t, before.Items[0].Evidence, after.Items[0].Evidence)
}

func TestReplacePlanAcceptsKnownStepTypes(t *testing.T) {
	m := NewManager(t.TempDir())
	plan, err := m.ReplacePlan([]PlanItem{
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

	_, err = m.ReplacePlan([]PlanItem{
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

func TestClearPlanResetsRevisionAndDropsItems(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)

	_, err = m.ReplacePlan([]PlanItem{{Content: "step", Status: PlanInProgress, Type: StepEdit}})
	require.NoError(t, err)

	cleared, err := m.ClearPlan()
	require.NoError(t, err)
	assert.Zero(t, cleared.Revision, "clear resets the revision counter")
	assert.Empty(t, cleared.Items, "clear drops every plan item")
	assert.False(t, cleared.Approved)

	// The empty snapshot must be the durable state, so a fresh plan can restart
	// from revision zero on the same session.
	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	restored := loaded.Plan()
	assert.Zero(t, restored.Revision)
	assert.Empty(t, restored.Items)
}

func TestReplacePlanUnapprovesOnStepUpdate(t *testing.T) {
	m := NewManager(t.TempDir())
	plan, err := m.ReplacePlan([]PlanItem{{Content: "step", Status: PlanInProgress, Type: StepEdit}})
	require.NoError(t, err)
	require.False(t, plan.Approved)

	approved, err := m.SetPlanApproved(true)
	require.NoError(t, err)
	require.True(t, approved.Approved)

	updated, err := m.ReplacePlan([]PlanItem{{Content: "step 2", Status: PlanInProgress, Type: StepEdit}})
	require.NoError(t, err)
	assert.False(t, updated.Approved, "any step update must drop approval")
}

func TestReplacePlanDropsApprovalWhenAllWorkCloses(t *testing.T) {
	m := NewManager(t.TempDir())
	_, err := m.ReplacePlan([]PlanItem{{Content: "step", Status: PlanInProgress, Type: StepEdit}})
	require.NoError(t, err)

	_, err = m.SetPlanApproved(true)
	require.NoError(t, err)

	updated, err := m.ReplacePlan([]PlanItem{{Content: "step", Status: PlanCompleted, Type: StepEdit}})
	require.NoError(t, err)
	assert.False(t, updated.Approved, "a plan with no active work must close approval")
}

func TestReplacePlanWithAutoApproveCommitsTruthfulSnapshot(t *testing.T) {
	m := NewManager(t.TempDir())

	active, err := m.ReplacePlanWithAutoApprove([]PlanItem{{
		Content: "step", Status: PlanInProgress, Type: StepEdit,
	}}, true)
	require.NoError(t, err)
	assert.True(t, active.Approved)
	assert.Equal(t, uint64(1), active.Revision, "replacement and approval must be one snapshot")

	closed, err := m.ReplacePlanWithAutoApprove([]PlanItem{{
		Content: "step", Status: PlanCancelled, Type: StepEdit,
	}}, true)
	require.NoError(t, err)
	assert.False(t, closed.Approved, "auto-approval must not revive a closed plan")
	assert.Equal(t, uint64(2), closed.Revision)
}
