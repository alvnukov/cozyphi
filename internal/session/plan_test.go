package session

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/llm"
)

func TestUpdatePlanDoesNotMoveConversationLeafOrContext(t *testing.T) {
	m := NewManager(t.TempDir())
	_, err := m.Append(llm.Message{Role: llm.RoleUser, Content: "keep me"})
	require.NoError(t, err)
	leaf := m.LeafID()
	before := messageContents(m.BuildContext())

	plan, err := m.UpdatePlan([]PlanItem{{Content: " inspect ", Status: PlanInProgress}})
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

	first, err := m.UpdatePlan([]PlanItem{{Content: "first", Status: PlanInProgress}})
	require.NoError(t, err)
	latest, err := m.UpdatePlan([]PlanItem{
		{Content: "first", Status: PlanCompleted},
		{Content: "second", Status: PlanPending},
	})
	require.NoError(t, err)
	require.Equal(t, first.Revision+1, latest.Revision)

	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	restored := loaded.Plan()
	assert.Equal(t, latest.Revision, restored.Revision)
	assert.Equal(t, latest.Items, restored.Items)
	assert.True(t, latest.UpdatedAt.Equal(restored.UpdatedAt))
	assert.Empty(t, loaded.LeafID(), "plan metadata must not become a conversation leaf")
	assert.Empty(t, loaded.BuildContext(), "plan metadata must stay out of provider context")
}

func TestUpdatePlanRejectsInvalidSnapshotsWithoutMutation(t *testing.T) {
	m := NewManager(t.TempDir())
	valid, err := m.UpdatePlan([]PlanItem{{Content: "one", Status: PlanPending}})
	require.NoError(t, err)

	cases := map[string][]PlanItem{
		"empty content":  {{Content: "  ", Status: PlanPending}},
		"invalid status": {{Content: "one", Status: "started"}},
		"two active": {
			{Content: "one", Status: PlanInProgress},
			{Content: "two", Status: PlanInProgress},
		},
		"too long": {{Content: strings.Repeat("x", maxPlanContentRunes+1), Status: PlanPending}},
	}
	for name, items := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := m.UpdatePlan(items)
			require.Error(t, err)
			assert.Equal(t, valid, m.Plan())
		})
	}
}

func TestUpdatePlanSerializesConcurrentSnapshots(t *testing.T) {
	m := NewManager(t.TempDir())
	const updates = 24
	errCh := make(chan error, updates)
	var wg sync.WaitGroup
	for range updates {
		wg.Go(func() {
			_, err := m.UpdatePlan([]PlanItem{{Content: "step", Status: PlanCompleted}})
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

func TestUpdatePlanPersistenceFailureDoesNotPublishState(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	m.sessionFile = dir // a directory cannot be atomically used as the JSONL file

	_, err = m.UpdatePlan([]PlanItem{{Content: "must fail", Status: PlanPending}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "persist plan")
	assert.Zero(t, m.Plan().Revision)
	assert.Empty(t, m.Plan().Items)
}
