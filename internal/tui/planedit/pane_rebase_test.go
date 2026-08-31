package planedit_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/planedit"
)

func TestPaneRebasesAndRetriesWhenThePlanMovesUnderTheModal(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan(), interfere: agentMoves(func(plan *session.Plan) {
		plan.Items[1].Status = session.PlanInProgress
	})}
	pane := newPane(store)
	editGoal(t, pane, " again")

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))

	require.Len(t, store.applied, 2, "the refused patch is retried on the newer revision")
	assert.Equal(t, uint64(4), store.applied[0].rev)
	assert.Equal(t, uint64(5), store.applied[1].rev)
	assert.Equal(t, store.applied[0].ops, store.applied[1].ops, "an untouched field needs no merge")
	assert.False(t, pane.Visible(), "a clean rebase commits without asking")
	assert.Equal(t, "ship the plan editor again", store.snapshot.Goal)
}

func TestPaneKeepsTheModalOpenAndNamesWhatTheRebaseTook(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan(), interfere: agentMoves(func(plan *session.Plan) {
		plan.Goal = "the agent rewrote the goal"
	})}
	pane := newPane(store)
	editGoal(t, pane, " again")
	down(t, pane, 2) // Context.
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	paste(pane, " updated")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))

	require.Len(t, store.applied, 1, "a conflicting rebase is never retried behind the user's back")
	assert.True(t, pane.Visible())
	assert.True(t, pane.State().Dirty)
	assert.Contains(t, pane.State().Error, "rev 5")
	assert.Contains(t, pane.State().Error, "goal")

	screen := renderText(t, pane, 100, 24)
	assert.Contains(t, screen, "rev 5", "the modal now sits on the newer revision")
	assert.Contains(t, screen, "the agent rewrote the goal")

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))

	require.Len(t, store.applied, 2)
	assert.Equal(t, uint64(5), store.applied[1].rev)
	require.Len(t, store.applied[1].ops, 1, "only the edit that survived is written")
	assert.Equal(t, session.PlanPatchReplaceContext, store.applied[1].ops[0].Op)
	assert.False(t, pane.Visible())
}

func TestPaneReportsAPlanThatKeepsMovingInsteadOfLoopingOnIt(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	store.err = &session.StalePlanRevisionError{Expected: 4, Actual: 5}
	pane := newPane(store)
	editGoal(t, pane, " again")

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))

	assert.Len(t, store.applied, 2, "the retry is bounded to one")
	assert.True(t, pane.Visible())
	assert.Contains(t, pane.State().Error, "still moving")
}

// agentMoves commits nothing and refuses the write: the plan the editor is
// about to read has already advanced.
func agentMoves(mutate func(*session.Plan)) func(*fakeStore, uint64) error {
	return func(store *fakeStore, rev uint64) error {
		mutate(&store.snapshot)
		store.snapshot.Revision++
		return &session.StalePlanRevisionError{Expected: rev, Actual: store.snapshot.Revision}
	}
}

func editGoal(t *testing.T, pane *planedit.Pane, suffix string) {
	t.Helper()
	require.True(t, key(pane, xui.KeyEnter, 0, 0)) // Goal is initially selected.
	paste(pane, suffix)
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
}
