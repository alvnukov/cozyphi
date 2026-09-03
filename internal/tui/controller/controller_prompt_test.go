package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func TestController_CancelKeepsRunActiveUntilLoopExits(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	ctrl := &Controller{
		streamCancel:  cancel,
		streamGen:     1,
		streamRunning: true,
		promptQueue:   []queuedPrompt{{text: "follow up"}},
	}

	ctrl.Cancel()

	assert.True(t, ctrl.streamRunning, "cancel must not permit a second concurrent loop")
	require.Len(t, ctrl.promptQueue, 1, "user cancel must preserve accepted prompts")
	assert.False(t, ctrl.Alive(1), "events arriving after cancellation must be ignored")
	select {
	case <-ctx.Done():
	default:
		t.Fatal("active loop context was not cancelled")
	}
}

func TestController_StartPromptSnapshotsPendingSkills(t *testing.T) {
	ctrl := &Controller{streamRunning: true, modelCfg: llm.ModelConfig{Name: "test-model"}}
	skills := []string{"review"}

	ctrl.StartPrompt("inspect", skills, "")
	skills[0] = "mutated"

	require.Len(t, ctrl.promptQueue, 1)
	assert.Equal(t, []string{"review"}, ctrl.promptQueue[0].pendingSkills)
}

func TestController_ShutdownCancelsRunDropsQueueAndRejectsNewPrompts(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	ctrl := &Controller{
		streamCancel:  cancel,
		streamGen:     1,
		streamRunning: true,
		promptQueue:   []queuedPrompt{{text: "not during shutdown"}},
	}

	ctrl.shutdownPrompts()
	ctrl.StartPrompt("also rejected", nil, "")

	assert.True(t, ctrl.closing)
	assert.True(t, ctrl.streamRunning, "shutdown waits for the active loop to exit")
	assert.Empty(t, ctrl.promptQueue)
	select {
	case <-ctx.Done():
	default:
		t.Fatal("shutdown did not cancel the active loop")
	}
}

func TestController_LifecycleMutationRequiresIdleRun(t *testing.T) {
	ctrl := &Controller{streamRunning: true, sessionDir: t.TempDir()}
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "set model", run: func() error { return ctrl.SetModel("other") }},
		{name: "resume", run: func() error {
			_, err := ctrl.Resume("session")
			return err
		}},
		{name: "clear", run: ctrl.Clear},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "reply or queued prompt is running")
		})
	}
}
