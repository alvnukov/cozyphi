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

// SessionEvents is synchronous: the save lands after one admitted action has
// finished, before the next action or the step-model switch is admitted.
func TestUserSaveBetweenLifecycleEffects(t *testing.T) {
	for _, door := range []string{"transition", "settle", "auto-start"} {
		for _, actionCount := range []int{1, 2} {
			t.Run(door+string(rune('0'+actionCount)), func(t *testing.T) {
				server, _ := fakeToolSequenceServer(1)
				defer server.Close()
				var engine *Engine
				var effectErr error
				var runs atomic.Int32
				events := 0
				tool := countingTool(&runs)
				tool.Run = func(ctx context.Context, _ json.RawMessage) (tools.Result, error) {
					switch door {
					case "transition":
						_, _, effectErr = engine.transitionPlan(
							ctx,
							session.PlanTransition{
								Action:     session.TransitionStart,
								StepID:     "explore",
								MutationID: "start",
							},
						)
					case "settle":
						effectErr = engine.settlePlanFromCall(
							ctx,
							session.PlanSettle{StartStepID: "explore", MutationID: "start"},
						)
					case "auto-start":
						effectErr = engine.autoStartStep(ctx, "explore")
					}
					return tools.Result{}, effectErr
				}
				var err error
				engine, err = NewEngine(EngineOpts{
					Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
					SessionOpts: SessionOpts{Cwd: t.TempDir()}, Tools: []tools.Tool{tool}, Gate: permission.AllowAll{},
					SessionEvents: func(event session.Event) {
						if _, ok := event.(session.PlanActionRan); ok {
							events++
							if events == 1 {
								saveUserGoal(t, engine, "saved between effects")
							}
						}
					},
				})
				require.NoError(t, err)
				engine.resolveModel = resolveOnly(server.URL)
				contract := seedContract()
				contract.Items[0].Model = "plan-b"
				for range actionCount {
					contract.Items[0].Actions = append(
						contract.Items[0].Actions,
						session.PlanAction{Event: session.PlanActionOnStepStart, Type: session.PlanActionCompact},
					)
				}
				seedApprovedActionPlan(t, engine, contract)
				engine.SetPlanGate(&plangate.Checker{Phase: plangate.PhaseHint})
				for _, err := range engine.Loop(t.Context(), "continue", LoopOpts{}) {
					require.NoError(t, err)
				}
				require.ErrorContains(t, effectErr, "stale model round")
				require.Equal(t, 1, events, "already admitted action completes; later actions must not start")
				require.Equal(t, "fake", engine.ModelConfig().Name, "stale start must not switch model")
				require.Equal(t, session.PlanPending, engine.Plan().Items[0].Status)
				require.Equal(t, "saved between effects", engine.Plan().Goal)
			})
		}
	}
}
