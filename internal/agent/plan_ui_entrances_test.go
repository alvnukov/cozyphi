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

func TestUIPlanEntrancesInvalidateRunningRound(t *testing.T) {
	for _, edit := range []string{"clear", "skill", "model"} {
		t.Run(edit, func(t *testing.T) {
			server, _ := fakeToolSequenceServer(1)
			defer server.Close()
			var engine *Engine
			var writeErr error
			var runs atomic.Int32
			tool := countingTool(&runs)
			tool.Run = func(ctx context.Context, _ json.RawMessage) (tools.Result, error) {
				switch edit {
				case "clear":
					_, err := engine.ClearPlan()
					require.NoError(t, err)
				case "model":
					require.NoError(t, engine.SetStepModel("explore", "pinned"))
				default:
					_, err := engine.SetPlanSkillDisabled("explore", 0, "review", true)
					require.NoError(t, err)
				}
				_, _, _, writeErr = engine.createPlan(ctx, seedContract())
				return tools.Result{}, writeErr
			}
			var err error
			engine, err = NewEngine(EngineOpts{
				Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
				SessionOpts: SessionOpts{Cwd: t.TempDir()}, Tools: []tools.Tool{tool}, Gate: permission.AllowAll{},
			})
			require.NoError(t, err)
			contract := seedContract()
			contract.Items[0].Actions = []session.PlanAction{
				{Event: session.PlanActionOnStepStart, Type: session.PlanActionInjectSkill, Skills: []string{"review"}},
			}
			_, _, _, err = engine.createPlan(t.Context(), contract)
			require.NoError(t, err)
			engine.SetPlanGate(&plangate.Checker{Phase: plangate.PhaseHint})
			for _, err := range engine.Loop(t.Context(), "continue", LoopOpts{}) {
				require.NoError(t, err)
			}
			require.ErrorContains(t, writeErr, "stale model round")
			switch edit {
			case "clear":
				require.Empty(t, engine.Plan().Items)
			case "model":
				require.Equal(t, "pinned", engine.Plan().Items[0].Model)
				require.False(t, engine.Plan().Approved)
			default:
				require.Contains(t, engine.Plan().Items[0].Actions[0].DisabledSkills, "review")
				require.False(t, engine.Plan().Approved)
			}
		})
	}
}
