package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

// RunActive is the one predicate gating user input: it must flip on
// synchronously with StartPrompt (covering the window before the first
// stream event) and stay up while prompts are queued behind a run.
func TestController_RunActiveTracksRuns(t *testing.T) {
	bus := NewBus(nil)
	ctrl := &Controller{bus: bus, modelCfg: llm.ModelConfig{Name: "test-model"}}

	assert.False(t, ctrl.RunActive())

	ctrl.StartPrompt("first", nil, "")
	assert.True(t, ctrl.RunActive())

	ctrl.StartPrompt("second", nil, "")
	assert.True(t, ctrl.RunActive(), "queued prompt keeps the pipeline busy")

	waitRunsFinished(t, bus, ctrl, 2)
	assert.False(t, ctrl.RunActive())
}

// RunEndedMsg fires once per busy period — not once per queued run — so the
// footer never flashes idle between chained prompts.
func TestController_RunEndedMsgFiresOncePerBusyPeriod(t *testing.T) {
	bus := NewBus(nil)
	ctrl := &Controller{bus: bus, modelCfg: llm.ModelConfig{Name: "test-model"}}

	ctrl.StartPrompt("first", nil, "")
	ctrl.StartPrompt("second", nil, "")

	ended := 0
	deadline := time.After(2 * time.Second)
	for ended == 0 {
		select {
		case <-bus.Chan():
			for _, msg := range bus.Drain() {
				if _, ok := msg.(RunEndedMsg); ok {
					ended++
				}
			}
		case <-deadline:
			t.Fatalf("RunEndedMsg count = %d, want 1", ended)
		}
	}
	require.Eventually(t, func() bool { return !ctrl.RunActive() }, time.Second, 2*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	for _, msg := range bus.Drain() {
		if _, ok := msg.(RunEndedMsg); ok {
			ended++
		}
	}
	assert.Equal(t, 1, ended)
}

// Compaction drives the footer itself now that nothing reconciles activity
// from the snapshot: it must announce ActivityCompacting and end with
// RunEndedMsg.
func TestController_CompactDrivesFooterActivity(t *testing.T) {
	bus := NewBus(nil)
	ctrl := &Controller{bus: bus}

	ctrl.Compact()

	compacting, ended := false, false
	deadline := time.After(2 * time.Second)
	for !ended {
		select {
		case <-bus.Chan():
			for _, msg := range bus.Drain() {
				switch m := msg.(type) {
				case SetActivityMsg:
					if m.Activity == ActivityCompacting {
						compacting = true
					}
				case RunEndedMsg:
					ended = true
				}
			}
		case <-deadline:
			t.Fatalf("compaction finished=%v without RunEndedMsg", ended)
		}
	}
	assert.True(t, compacting)
}

// waitRunsFinished drains the bus until `runs` runs emitted their terminal
// error event (nil engine) and the pipeline reports idle. The nil engine's
// "agent not configured" error stands in for a finished run.
func waitRunsFinished(t *testing.T, bus *Bus, ctrl *Controller, runs int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	finished := 0
	for finished < runs {
		select {
		case <-bus.Chan():
			for _, msg := range bus.Drain() {
				event, ok := msg.(SessionEventMsg)
				if !ok {
					continue
				}
				if update, ok := event.Event.(session.AssistantMessageUpdate); ok &&
					update.Message.State == session.StateError {
					finished++
				}
			}
		case <-deadline:
			t.Fatalf("finished runs = %d, want %d", finished, runs)
		}
	}
	require.Eventually(t, func() bool { return !ctrl.RunActive() }, time.Second, 2*time.Millisecond)
}
