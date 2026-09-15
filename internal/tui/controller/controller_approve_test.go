package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
)

func newReadyController(t *testing.T) *Controller {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")

	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())

	ctrl, err := NewController(NewBus(nil), proj, cwd, "")
	require.NoError(t, err)
	require.NotNil(t, ctrl.engine)
	return ctrl
}

func TestController_SetPlanApprovedAllowsApproveMidStream(t *testing.T) {
	ctrl := newReadyController(t)
	ctx, cancel := context.WithCancel(t.Context())
	ctrl.streamMu.Lock()
	ctrl.streamRunning = true
	ctrl.streamCancel = cancel
	ctrl.promptQueue = []queuedPrompt{{text: "follow up"}}
	ctrl.streamMu.Unlock()

	require.NoError(t, ctrl.SetPlanApproved(true))
	assert.True(t, ctrl.Plan().Approved)

	select {
	case <-ctx.Done():
		t.Fatal("approving must not cancel the in-flight stream")
	default:
	}
	assert.False(t, ctrl.streamStopped, "approving must not stop the stream")
	assert.Len(t, ctrl.promptQueue, 1, "approving must keep queued prompts")
}

func TestController_ClearPlanResetsRevisionAndRepublishes(t *testing.T) {
	ctrl := newReadyController(t)
	_, err := ctrl.engine.SetPlanApproved(true)
	require.NoError(t, err)
	require.NotZero(t, ctrl.Plan().Revision)

	require.NoError(t, ctrl.ClearPlan())
	plan := ctrl.Plan()
	assert.Zero(t, plan.Revision, "clear resets the revision counter")
	assert.Empty(t, plan.Items)
	assert.False(t, plan.Approved)
	assert.False(t, ctrl.planGateBlocked)
	assert.False(t, ctrl.planApprovalResumePending)
}

func TestController_ObserveToolDataTracksPlanGateBlock(t *testing.T) {
	c := &Controller{}

	c.observeToolData(session.ToolData{Run: session.ToolRun{
		Name: "bash", Status: session.ToolRejected, Error: plangate.ReasonPlanNotApproved,
	}})
	assert.True(t, c.planGateBlocked)

	c.observeToolData(session.ToolData{Run: session.ToolRun{Name: "bash", Status: session.ToolDone}})
	assert.False(t, c.planGateBlocked, "a successful gateable tool clears the block")

	c.observeToolData(session.ToolData{Run: session.ToolRun{
		Name: "bash", Status: session.ToolRejected, Error: plangate.ReasonPlanNotApproved,
	}})
	assert.True(t, c.planGateBlocked)
	c.observeToolData(session.ToolData{Run: session.ToolRun{Name: "plan", Status: session.ToolDone}})
	assert.True(t, c.planGateBlocked, "exempt tools never clear the block")
}

func TestControllerFinishRunResumesBlockedApprovedTurn(t *testing.T) {
	ctrl := newReadyController(t)
	_, err := ctrl.engine.SetPlanApproved(true)
	require.NoError(t, err)
	ctrl.streamMu.Lock()
	ctrl.planGateBlocked = true
	ctrl.streamRunning = true
	ctrl.streamGen = 7
	ctrl.streamMu.Unlock()

	ctrl.finishRun(7)

	assert.True(t, ctrl.streamRunning, "finishRun resumes a blocked, approved turn")
	assert.False(t, ctrl.planGateBlocked, "resume clears the blocked flag")
	ctrl.Cancel()
}

func TestController_SetPlanApprovedResumesBlockedTurn(t *testing.T) {
	ctrl := newReadyController(t)
	ctrl.planGateBlocked = true

	require.NoError(t, ctrl.SetPlanApproved(true))
	assert.True(t, ctrl.Plan().Approved)
	assert.True(t, ctrl.streamRunning, "approval must resume a turn blocked on the unapproved plan")
	assert.False(t, ctrl.planGateBlocked, "resume must clear the blocked flag")

	ctrl.Cancel()
}

func TestController_SetPlanApprovedResumesIdleTurnForActivePlan(t *testing.T) {
	ctrl := newReadyController(t)
	_, err := ctrl.engine.Session().ReplacePlan(t.Context(), []session.PlanItem{{
		Content: "implement approved work",
		Status:  session.PlanInProgress,
		Type:    session.StepEdit,
	}}, false)
	require.NoError(t, err)
	require.False(t, ctrl.Plan().Approved)

	require.NoError(t, ctrl.SetPlanApproved(true))
	assert.True(t, ctrl.Plan().Approved)
	assert.True(t, ctrl.RunActive(), "approval must hand an idle active plan back to the agent")

	ctrl.Cancel()
}

func TestController_SetPlanApprovedResumesIdleTurnForPendingOnlyPlan(t *testing.T) {
	ctrl := newReadyController(t)
	defer ctrl.Cancel()
	_, err := ctrl.engine.Session().ReplacePlan(t.Context(), []session.PlanItem{{
		Content: "drafted work waiting for approval",
		Status:  session.PlanPending,
		Type:    session.StepEdit,
	}}, false)
	require.NoError(t, err)
	require.False(t, ctrl.Plan().Approved)

	require.NoError(t, ctrl.SetPlanApproved(true))
	assert.True(t, ctrl.Plan().Approved)
	assert.True(t, ctrl.RunActive(), "approval must hand an idle all-pending plan back to the agent")
}

func TestController_SetPlanApprovedDoesNotResumeCompletedPlan(t *testing.T) {
	ctrl := newReadyController(t)
	_, err := ctrl.engine.Session().ReplacePlan(t.Context(), []session.PlanItem{{
		Content:  "finished work",
		Status:   session.PlanCompleted,
		Type:     session.StepEdit,
		Evidence: "tests passed",
	}}, false)
	require.NoError(t, err)

	require.NoError(t, ctrl.SetPlanApproved(true))
	assert.True(t, ctrl.Plan().Approved)
	assert.False(t, ctrl.RunActive(), "approval must not restart a completed plan")
}

func TestController_FinishRunDoesNotResumePlanCompletedWhileApprovalPending(t *testing.T) {
	ctrl := newReadyController(t)
	defer ctrl.Cancel()
	_, err := ctrl.engine.Session().ReplacePlan(t.Context(), []session.PlanItem{{
		Content: "finish approved work",
		Status:  session.PlanInProgress,
		Type:    session.StepEdit,
	}}, false)
	require.NoError(t, err)
	ctrl.streamMu.Lock()
	ctrl.streamRunning = true
	ctrl.streamGen = 7
	ctrl.streamMu.Unlock()
	require.NoError(t, ctrl.SetPlanApproved(true))

	_, err = ctrl.engine.Session().ReplacePlan(t.Context(), []session.PlanItem{{
		Content:  "finish approved work",
		Status:   session.PlanCompleted,
		Type:     session.StepEdit,
		Evidence: "completed before the stream ended",
	}}, false)
	require.NoError(t, err)
	ctrl.finishRun(7)

	assert.False(t, ctrl.RunActive(), "a completed plan must not resume after its current stream finishes")
}

func TestController_SetPlanApprovedDoesNotReuseBlockedResumeAfterUnapprove(t *testing.T) {
	ctrl := newReadyController(t)
	defer ctrl.Cancel()
	_, err := ctrl.engine.Session().ReplacePlan(t.Context(), []session.PlanItem{{
		Content: "finish blocked work",
		Status:  session.PlanInProgress,
		Type:    session.StepEdit,
	}}, false)
	require.NoError(t, err)
	ctrl.planGateBlocked = true
	require.NoError(t, ctrl.SetPlanApproved(false))

	_, err = ctrl.engine.Session().ReplacePlan(t.Context(), []session.PlanItem{{
		Content:  "finish blocked work",
		Status:   session.PlanCompleted,
		Type:     session.StepEdit,
		Evidence: "completed while unapproved",
	}}, false)
	require.NoError(t, err)
	require.NoError(t, ctrl.SetPlanApproved(true))

	assert.False(t, ctrl.RunActive(), "unapproval must clear a stale blocked-turn resume")
}

func TestController_SetPlanApprovedUnapproveStopsStream(t *testing.T) {
	ctrl := newReadyController(t)
	require.NoError(t, ctrl.SetPlanApproved(true))
	assert.True(t, ctrl.Plan().Approved)

	ctx, cancel := context.WithCancel(t.Context())
	ctrl.streamMu.Lock()
	ctrl.streamCancel = cancel
	ctrl.streamRunning = true
	ctrl.promptQueue = []queuedPrompt{{text: "follow up"}}
	ctrl.streamMu.Unlock()

	require.NoError(t, ctrl.SetPlanApproved(false))
	select {
	case <-ctx.Done():
	default:
		t.Fatal("unapprove must cancel the in-flight stream")
	}
	assert.True(t, ctrl.streamStopped)
	assert.Empty(t, ctrl.promptQueue, "unapprove must drop queued prompts")
	assert.False(t, ctrl.Plan().Approved)
}

// TestController_UnapproveDropsQueueWithoutPromote: dropping the queue on
// plan unapproval must NOT emit UserPromoted — those prompts never reached
// the model, and promote means "delivered". The queue widget is told the
// queue is empty, and that is all.
func TestController_UnapproveDropsQueueWithoutPromote(t *testing.T) {
	ctrl := newReadyController(t)
	require.NoError(t, ctrl.SetPlanApproved(true))

	ctrl.streamMu.Lock()
	ctrl.streamCancel = func() {}
	ctrl.streamRunning = true
	ctrl.promptQueue = []queuedPrompt{{text: "a", id: "u1"}, {text: "b", id: "u2"}}
	ctrl.streamMu.Unlock()

	require.NoError(t, ctrl.SetPlanApproved(false))

	var promoted bool
	var queueEmptied bool
	for _, msg := range ctrl.bus.Drain() {
		switch m := msg.(type) {
		case SessionEventMsg:
			if _, ok := m.Event.(session.UserPromoted); ok {
				promoted = true
			}
		case PromptQueueMsg:
			if len(m.Items) == 0 {
				queueEmptied = true
			}
		}
	}
	assert.False(t, promoted, "dropped prompts were never delivered — no UserPromoted")
	assert.True(t, queueEmptied, "the queue widget must learn the queue is empty")
}
