package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/project"
)

func newReadyController(t *testing.T) *Controller {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PHI_MODEL", "test-model")
	t.Setenv("PHI_API_KEY", "test-key")
	t.Setenv("PHI_BASE_URL", "http://127.0.0.1:9")

	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())

	ctrl, err := NewController(NewBus(nil), proj, cwd, "")
	require.NoError(t, err)
	require.NotNil(t, ctrl.engine)
	return ctrl
}

func TestController_SetPlanApprovedRejectsApproveMidStream(t *testing.T) {
	ctrl := newReadyController(t)
	ctrl.streamRunning = true

	err := ctrl.SetPlanApproved(true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot approve")
}

func TestController_SetPlanApprovedUnapproveStopsStream(t *testing.T) {
	ctrl := newReadyController(t)
	require.NoError(t, ctrl.SetPlanApproved(true))
	assert.True(t, ctrl.Plan().Approved)

	ctx, cancel := context.WithCancel(t.Context())
	ctrl.streamCancel = cancel
	ctrl.streamRunning = true
	ctrl.promptQueue = []queuedPrompt{{text: "follow up"}}

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
