package agent

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

func saveUserGoal(t *testing.T, engine *Engine, goal string) {
	t.Helper()
	_, _, err := engine.PatchPlanFromUser(
		t.Context(),
		engine.Plan().Revision,
		[]session.PlanPatchOp{
			{Op: session.PlanPatchSetPlanFields, Goal: session.PatchValue[string]{Set: true, Value: goal}},
		},
	)
	require.NoError(t, err)
}

func TestUserPlanSaveDuringPermissionAskRejectsDispatch(t *testing.T) {
	server, _ := fakeToolSequenceServer(1)
	defer server.Close()
	var runs atomic.Int32
	var engine *Engine
	var err error
	asked := false
	engine, err = NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Tools:       []tools.Tool{countingTool(&runs)},
		Gate:        fixedGate{dec: permission.Ask},
		Ask: func(context.Context, permission.Request, string) (permission.AskResult, error) {
			asked = true
			saveUserGoal(t, engine, "saved while asking")
			return permission.AskResult{Approved: true}, nil
		},
	})
	require.NoError(t, err)
	_, _, _, err = engine.createPlan(t.Context(), seedContract())
	require.NoError(t, err)
	engine.SetPlanGate(&plangate.Checker{Phase: plangate.PhaseHint})
	for _, err := range engine.Loop(t.Context(), "continue", LoopOpts{}) {
		require.NoError(t, err)
	}
	require.True(t, asked)
	require.Zero(t, runs.Load())
	require.Equal(t, "saved while asking", engine.Plan().Goal)
}

// Saving from inside Run models the race after Executor's last check. The
// session write must reject even though dispatch already passed every gate.
func TestUserPlanSaveGuardsActualPlanWrite(t *testing.T) {
	for _, action := range []string{"patch", "create", "update", "settle"} {
		t.Run(action, func(t *testing.T) {
			server, _ := fakeToolSequenceServer(1)
			defer server.Close()
			var engine *Engine
			var writeErr error
			var runs atomic.Int32
			tool := countingTool(&runs)
			tool.Run = func(ctx context.Context, _ json.RawMessage) (tools.Result, error) {
				saveUserGoal(t, engine, "user won the race")
				switch action {
				case "patch":
					_, _, writeErr = engine.PatchPlan(
						ctx,
						engine.Plan().Revision,
						[]session.PlanPatchOp{
							{
								Op:   session.PlanPatchSetPlanFields,
								Goal: session.PatchValue[string]{Set: true, Value: "stale"},
							},
						},
					)
				case "create":
					_, _, _, writeErr = engine.createPlan(ctx, seedContract())
				case "update":
					_, writeErr = engine.updatePlan(ctx, seedContract().Items)
				case "settle":
					working := "stale working context"
					_, _, writeErr = engine.Session().
						SettlePlanFromCall(ctx, session.PlanSettle{MutationID: "late", WorkingContext: &working})
				}
				return tools.Result{}, writeErr
			}
			var err error
			engine, err = NewEngine(
				EngineOpts{
					Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
					SessionOpts: SessionOpts{Cwd: t.TempDir()},
					Tools:       []tools.Tool{tool},
					Gate:        permission.AllowAll{},
				},
			)
			require.NoError(t, err)
			_, _, _, err = engine.createPlan(t.Context(), seedContract())
			require.NoError(t, err)
			engine.SetPlanGate(&plangate.Checker{Phase: plangate.PhaseHint})
			for _, err := range engine.Loop(t.Context(), "continue", LoopOpts{}) {
				require.NoError(t, err)
			}
			require.ErrorContains(t, writeErr, "stale model round")
			require.Equal(t, "user won the race", engine.Plan().Goal)
			require.Empty(t, engine.Plan().WorkingContext)
		})
	}
}
