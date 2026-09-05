package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestEffortLifecycleRestoresAndResumes(t *testing.T) {
	for _, ending := range []string{session.TransitionCancel, session.TransitionComplete, "settle"} {
		t.Run(ending, func(t *testing.T) {
			server, _, _ := fakeContextServer(t, "", func(int32) string { return "" })
			engine := newContextTestEngine(t, server.URL, 100000)
			base, ok := resolveOnly(server.URL)("plan-b")
			require.True(t, ok)
			require.NoError(t, engine.SetModel(base))
			seedApprovedActionPlan(t, engine, session.PlanV2{
				Goal: "work", Approach: "isolate effort", SuccessCriteria: []string{"done"},
				Items: []session.PlanItem{
					{ID: "a", Content: "A", Type: session.StepEdit, Why: "needed", DoneWhen: "done", Effort: "high"},
					{ID: "b", Content: "B", Type: session.StepEdit, Why: "needed", DoneWhen: "done", Effort: "low"},
				},
			})
			move := func(action, id string) {
				t.Helper()
				tr := session.PlanTransition{Action: action, StepID: id, MutationID: session.NewMutationID()}
				switch action {
				case session.TransitionBlock:
					tr.Blocker, tr.ResumeWhen = "waiting", "ready"
				case session.TransitionComplete:
					tr.Outcome, tr.Evidence = "done", "verified"
				case session.TransitionCancel:
					tr.Reason = "not needed"
				}
				_, _, err := engine.transitionPlan(t.Context(), tr)
				require.NoError(t, err)
			}
			move(session.TransitionStart, "a")
			require.Equal(t, llm.ReasoningEffortHigh, engine.ModelConfig().ReasoningEffort)
			move(session.TransitionBlock, "a")
			require.Equal(t, base, engine.ModelConfig())
			move(session.TransitionStart, "b")
			require.Equal(t, llm.ReasoningEffortLow, engine.ModelConfig().ReasoningEffort)
			move(session.TransitionComplete, "b")
			require.Equal(t, base, engine.ModelConfig())
			move(session.TransitionResume, "a")
			require.Equal(t, llm.ReasoningEffortHigh, engine.ModelConfig().ReasoningEffort)
			if ending == "settle" {
				require.NoError(t, engine.settlePlanFromCall(t.Context(), session.PlanSettle{
					MutationID: session.NewMutationID(),
					Complete: &session.PlanTransition{
						Action:   session.TransitionComplete,
						StepID:   "a",
						Outcome:  "done",
						Evidence: "verified",
					},
				}))
			} else {
				move(ending, "a")
			}
			require.Equal(t, base, engine.ModelConfig())
		})
	}
}
