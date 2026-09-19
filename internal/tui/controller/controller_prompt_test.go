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

	ctrl.StartPrompt("inspect", skills)
	skills[0] = "mutated"

	require.Len(t, ctrl.promptQueue, 1)
	assert.Equal(t, []string{"review"}, ctrl.promptQueue[0].pendingSkills)
}

// TestController_RecallQueuedPromptPopsNewestFirst: Esc recall walks the
// queue from the newest entry down, one per call, and leaves the order of
// the remaining entries intact for the drain that delivers them in turn.
func TestController_RecallQueuedPromptPopsNewestFirst(t *testing.T) {
	ctrl := &Controller{
		promptQueue: []queuedPrompt{
			{text: "first queued", id: "u1"},
			{text: "second queued", id: "u2"},
		},
	}

	text, _, _, ok := ctrl.RecallQueuedPrompt()
	require.True(t, ok)
	assert.Equal(t, "second queued", text)

	text, _, _, ok = ctrl.RecallQueuedPrompt()
	require.True(t, ok)
	assert.Equal(t, "first queued", text)
	assert.Empty(t, ctrl.promptQueue, "both entries must be popped by now")

	_, _, _, ok = ctrl.RecallQueuedPrompt()
	assert.False(t, ok, "empty queue has nothing to recall")
}

func TestController_RecallKeepsEarlierQueueOrder(t *testing.T) {
	ctrl := &Controller{
		promptQueue: []queuedPrompt{
			{text: "a", id: "u1"},
			{text: "b", id: "u2"},
			{text: "c", id: "u3"},
		},
	}

	_, _, _, ok := ctrl.RecallQueuedPrompt()
	require.True(t, ok)

	require.Len(t, ctrl.promptQueue, 2)
	assert.Equal(t, "a", ctrl.promptQueue[0].text)
	assert.Equal(t, "b", ctrl.promptQueue[1].text, "recall must not reorder what is left")
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
	ctrl.StartPrompt("also rejected", nil)

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

// TestController_CutShortTurnRequeuesOpeningPrompt: a queued prompt leaves the
// queue before its turn begins, so a turn cut short before the engine ever
// promoted it must hand it back. The strip is the prompt's only trace — there
// is no transcript row yet — so dropping it here loses the user's words with
// nothing on screen to show for them.
func TestController_CutShortTurnRequeuesOpeningPrompt(t *testing.T) {
	ctrl := &Controller{bus: NewBus(nil), streamGen: 1, streamRunning: true}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	ctrl.runLoop(ctx, 1, nil, runRequest{pending: queuedPrompt{text: "keep me", id: "u1", rowOwed: true}})

	require.Len(t, ctrl.promptQueue, 1)
	assert.Equal(t, "keep me", ctrl.promptQueue[0].text)
}

// TestController_FailedTurnDoesNotRequeue: an error row is the turn's answer to
// the prompts it holds. Handing them back would put them straight into an
// identical turn and loop on the same failure forever.
func TestController_FailedTurnDoesNotRequeue(t *testing.T) {
	ctrl := &Controller{
		bus:           NewBus(nil),
		streamGen:     1,
		streamRunning: true,
		modelCfg:      llm.ModelConfig{Name: "test-model"},
	}

	ctrl.runLoop(t.Context(), 1, nil, runRequest{pending: queuedPrompt{text: "boom", id: "u1", rowOwed: true}})

	assert.Empty(t, ctrl.promptQueue, "the failure is reported once, not retried forever")
}

// TestController_RequeueLockedKeepsOnlyPromptsOwedARow: only a prompt still
// owed a transcript row goes back. A watch wake, a plan resume and a submit
// whose row the submitter already published owe no row, and requeuing those
// would run them a second time.
func TestController_RequeueLockedKeepsOnlyPromptsOwedARow(t *testing.T) {
	ctrl := &Controller{bus: NewBus(nil), promptQueue: []queuedPrompt{{text: "waiting", id: "u3", rowOwed: true}}}

	ctrl.requeueLocked(
		queuedPrompt{text: "wake"},
		queuedPrompt{text: "first", id: "u1", rowOwed: true},
		queuedPrompt{text: "second", id: "u2", rowOwed: true},
	)

	require.Len(t, ctrl.promptQueue, 3)
	assert.Equal(t, []string{"first", "second", "waiting"}, []string{
		ctrl.promptQueue[0].text, ctrl.promptQueue[1].text, ctrl.promptQueue[2].text,
	}, "requeued prompts go back in front, in order, ahead of what is still waiting")

	ctrl.closing = true
	ctrl.requeueLocked(queuedPrompt{text: "too late", id: "u4", rowOwed: true})
	assert.Len(t, ctrl.promptQueue, 3, "a closing session takes nothing back")
}
