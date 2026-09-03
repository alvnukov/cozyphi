package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// jitHandoffFixture wires an executor against one approved plan step the
// test shapes. The ask handler (installed per test on ex.ask, exactly as
// the engine does) plays the user; the SetPlanGate callbacks play the
// durable grant, the auto-start and the attempt recorder.
type jitHandoffFixture struct {
	ex        *Executor
	plan      session.Plan
	grantErr  error
	grants    []string // step grants the harness recorded
	started   []string
	recorded  []recordedAttempt
	reason    string // the question the user was asked
	rejectErr string
	ran       atomic.Int32
}

func newJITHandoffFixture(t *testing.T, status session.PlanStatus) *jitHandoffFixture {
	t.Helper()
	f := &jitHandoffFixture{
		plan: session.Plan{
			Revision: 4, Approved: true, ContractEpoch: 2,
			Items: []session.PlanItem{{
				ID: "push-tag", Content: "push the release tag", Status: status,
				Type: session.StepRun, Risk: "a published tag is irreversible", JIT: true,
			}},
		},
	}
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				f.ran.Add(1)
				return tools.Result{Content: "ran"}, nil
			},
		},
	}
	f.ex = NewExecutor(reg, permission.AllowAll{}, nil, nil)
	f.ex.SetPlanGate(
		&plangate.Checker{Phase: plangate.PhaseDeny},
		func() session.Plan { return f.plan },
		func(_ context.Context, stepID string) error {
			f.started = append(f.started, stepID)
			f.plan.Items[0].Status = session.PlanInProgress
			return nil
		},
		nil,
		func(stepID string, attempt session.PlanAttempt) error {
			f.recorded = append(f.recorded, recordedAttempt{stepID: stepID, attempt: attempt})
			return nil
		},
		func(stepID string, granted bool) error {
			if err := f.grantErr; err != nil {
				return err
			}
			f.grants = append(f.grants, fmt.Sprintf("%s:%t", stepID, granted))
			return nil
		},
	)
	return f
}

// approvingAsk plays the user who reads the demand and says yes.
func (f *jitHandoffFixture) approvingAsk() permission.AskFunc {
	return func(_ context.Context, _ permission.Request, reason string) (permission.AskResult, error) {
		f.reason = reason
		return permission.AskResult{Approved: true}, nil
	}
}

func (f *jitHandoffFixture) run(t *testing.T) []llm.Message {
	t.Helper()
	msgs, _, _ := f.ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"git push","plan_step":"push-tag"}`},
	}}, func(td session.ToolData) bool {
		if td.Run.Status == session.ToolRejected {
			f.rejectErr = td.Run.Error
		}
		return true
	})
	return msgs
}

func TestJITAskApproveRecordsGrantAndRuns(t *testing.T) {
	f := newJITHandoffFixture(t, session.PlanInProgress)
	f.ex.ask = f.approvingAsk()

	msgs := f.run(t)
	require.Len(t, msgs, 1)
	assert.Equal(t, int32(1), f.ran.Load(), "the approved call runs")
	assert.Equal(t, []string{"push-tag:true"}, f.grants, "approval records the durable grant")
	assert.Contains(t, f.reason, "push the release tag", "the user sees the action")
	assert.Contains(t, f.reason, "a published tag is irreversible", "the user sees the risk")
	assert.Empty(t, f.rejectErr)
	assert.Len(t, f.recorded, 1, "the accepted call files its attempt")
}

func TestJITAskDenyRejectsWithHumanDiff(t *testing.T) {
	f := newJITHandoffFixture(t, session.PlanPending)
	f.ex.ask = func(
		_ context.Context, _ permission.Request, _ string,
	) (permission.AskResult, error) {
		return permission.AskResult{Approved: false, Feedback: "not yet"}, nil
	}

	msgs := f.run(t)
	require.Len(t, msgs, 1)
	assert.Equal(t, int32(0), f.ran.Load(), "the denied call never runs")
	assert.Contains(t, f.rejectErr, "push-tag")
	assert.Contains(t, f.rejectErr, "push the release tag")
	assert.Contains(t, f.rejectErr, "a published tag is irreversible")
	assert.Contains(t, f.rejectErr, "not yet")
	assert.Empty(t, f.started, "a denied call never starts the step")
	assert.Empty(t, f.recorded, "a rejected call files no attempt")
	assert.Empty(t, f.grants, "a denial records no grant")
}

func TestJITNoAskHandlerFailsClosed(t *testing.T) {
	f := newJITHandoffFixture(t, session.PlanInProgress)

	msgs := f.run(t)
	require.Len(t, msgs, 1)
	assert.Equal(t, int32(0), f.ran.Load())
	assert.Contains(t, f.rejectErr, "just-in-time")
	assert.Empty(t, f.grants)
}

func TestNonJITCallSkipsTheAsk(t *testing.T) {
	f := newJITHandoffFixture(t, session.PlanInProgress)
	f.ex.ask = func(
		_ context.Context, _ permission.Request, _ string,
	) (permission.AskResult, error) {
		t.Fatal("a step without the marker never asks")
		return permission.AskResult{}, nil
	}
	f.plan.Items[0].JIT = false

	msgs := f.run(t)
	require.Len(t, msgs, 1)
	assert.Equal(t, int32(1), f.ran.Load(), "non-JIT steps run exactly as before")
	assert.Empty(t, f.grants)
}

func TestJITGrantedCallRunsWithoutAsk(t *testing.T) {
	f := newJITHandoffFixture(t, session.PlanInProgress)
	f.ex.ask = func(
		_ context.Context, _ permission.Request, _ string,
	) (permission.AskResult, error) {
		t.Fatal("a grant at the current epoch satisfies the demand")
		return permission.AskResult{}, nil
	}
	f.plan.JITApprovals = []session.JITApproval{{StepID: "push-tag", Epoch: 2}}

	msgs := f.run(t)
	require.Len(t, msgs, 1)
	assert.Equal(t, int32(1), f.ran.Load())
	assert.Empty(t, f.grants, "no new grant is recorded for an already-granted step")
}

func TestJITGrantRecordFailureStillRunsApprovedCall(t *testing.T) {
	f := newJITHandoffFixture(t, session.PlanInProgress)
	f.ex.ask = f.approvingAsk()
	f.grantErr = errors.New("disk full")

	msgs := f.run(t)
	require.Len(t, msgs, 1)
	assert.Equal(t, int32(1), f.ran.Load(), "the user said yes; the call runs")
	assert.Contains(t, msgs[0].Content, "just-in-time approval was not recorded")
}

func TestJITPendingStepStartsOnlyAfterApproval(t *testing.T) {
	f := newJITHandoffFixture(t, session.PlanPending)
	f.ex.ask = f.approvingAsk()

	msgs := f.run(t)

	// Approval precedes the auto-start: the grant covers the step the call
	// then starts. The mirror test below proves the ordering without the
	// approval: a permission denial must not even reach the JIT ask.
	require.Len(t, msgs, 1)
	assert.Equal(t, int32(1), f.ran.Load())
	assert.Equal(t, []string{"push-tag:true"}, f.grants)
	assert.Equal(t, []string{"push-tag"}, f.started)
}

// TestJITPermissionDenialSkipsTheAsk pins the gate order the executor
// promises: permission runs before the JIT handoff, so a denial asks no
// approval question, records no grant, starts nothing and dispatches nothing.
func TestJITPermissionDenialSkipsTheAsk(t *testing.T) {
	f := newJITHandoffFixture(t, session.PlanInProgress)
	f.ex.ask = func(
		_ context.Context, _ permission.Request, _ string,
	) (permission.AskResult, error) {
		t.Fatal("a permission denial must not reach the JIT ask")
		return permission.AskResult{}, nil
	}
	f.ex.gate = denyGate{}
	f.ex.syncHookFilter()

	msgs := f.run(t)

	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "locked down")
	assert.Zero(t, f.ran.Load(), "no dispatch")
	assert.Empty(t, f.grants, "no grant")
	assert.Empty(t, f.recorded, "no attempt")
	assert.Empty(t, f.started, "nothing starts")
}
