package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestUserPlanReminderTracksOnlyCommittedUserChanges(t *testing.T) {
	for _, kind := range []string{"model", "noop", "failed", "user-create", "coalesced", "replacement", "compacted", "postcommit-failure", "bounded", "model-settings"} {
		t.Run(kind, func(t *testing.T) {
			server, _, bodies := fakeContextServer(t, "unused", func(int32) string { return sseTextChunk() })
			engine, err := NewEngine(
				EngineOpts{
					Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
					SessionOpts: SessionOpts{Cwd: t.TempDir()},
				},
			)
			require.NoError(t, err)
			contract := seedContract()
			if kind == "model-settings" {
				contract.ModelsByType = map[session.StepType]string{session.StepExplore: "private-type-model"}
				contract.Items[0].Model = "private-step-model"
				contract.Items[0].Effort = "medium"
				contract.Actions = []session.PlanAction{
					{Event: session.PlanActionOnPlanStart, Type: session.PlanActionCompact},
				}
			}
			if kind == "user-create" {
				_, _, _, err = engine.CreatePlan(t.Context(), contract)
			} else {
				_, _, _, err = engine.createPlan(t.Context(), contract)
			}
			require.NoError(t, err)
			switch kind {
			case "model-settings":
				saveUserGoal(t, engine, "user goal with private settings")
			case "model":
				_, _, err = engine.PatchPlan(
					t.Context(),
					engine.Plan().Revision,
					[]session.PlanPatchOp{
						{
							Op:   session.PlanPatchSetPlanFields,
							Goal: session.PatchValue[string]{Set: true, Value: "model authored"},
						},
					},
				)
				require.NoError(t, err)
			case "noop":
				saveUserGoal(t, engine, contract.Goal)
			case "failed":
				_, _, err = engine.PatchPlanFromUser(
					t.Context(),
					engine.Plan().Revision+1,
					[]session.PlanPatchOp{
						{
							Op:   session.PlanPatchSetPlanFields,
							Goal: session.PatchValue[string]{Set: true, Value: "failed save"},
						},
					},
				)
				require.Error(t, err)
			case "bounded":
				_, _, err = engine.PatchPlanFromUser(
					t.Context(),
					engine.Plan().Revision,
					[]session.PlanPatchOp{
						{
							Op:             session.PlanPatchReplaceContext,
							WorkingContext: session.PatchValue[string]{Set: true, Value: strings.Repeat("界", 5000)},
						},
					},
				)
				require.NoError(t, err)
			case "postcommit-failure":
				_, err = engine.SetPlanApproved(true)
				require.NoError(t, err)
				_, _, err = engine.Session().
					TransitionPlan(t.Context(), session.PlanTransition{Action: session.TransitionStart, StepID: "explore", MutationID: "start"}, false)
				require.NoError(t, err)
				_, _, err = engine.PatchPlanFromUser(t.Context(), engine.Plan().Revision, []session.PlanPatchOp{
					{
						Op:   session.PlanPatchSetPlanFields,
						Goal: session.PatchValue[string]{Set: true, Value: "committed before failure"},
					},
					{
						Op:    session.PlanPatchUpdateStep,
						ID:    "explore",
						Model: session.PatchValue[string]{Set: true, Value: "missing-model"},
					},
				})
				require.Error(t, err)
				require.Equal(t, "committed before failure", engine.Plan().Goal)
			case "coalesced", "replacement", "compacted":
				saveUserGoal(t, engine, "first user save")
				saveUserGoal(t, engine, "latest user save")
				if kind == "compacted" {
					require.NoError(
						t,
						engine.Session().
							AppendCompaction(session.Compaction{Summary: "Earlier model decisions were different."}),
					)
				}
				if kind == "replacement" {
					require.NoError(t, engine.ReplaceSession(SessionOpts{Cwd: t.TempDir()}))
				}
			}
			for _, err := range engine.Loop(t.Context(), "continue", LoopOpts{}) {
				require.NoError(t, err)
			}
			request := bodies()[0]
			if kind == "model-settings" {
				notice := requestMessageText(t, request, true)
				require.NotContains(t, notice, "private-type-model")
				require.NotContains(t, notice, "private-step-model")
				require.NotContains(t, notice, `"actions"`)
				require.Contains(t, notice, `"effort":"medium"`)
			}
			if kind == "bounded" {
				notice := requestMessageText(t, request, true)
				require.Less(t, len(notice), 13000)
				require.Contains(t, notice, "use plan get before further work")
			}
			if kind == "coalesced" || kind == "user-create" || kind == "compacted" || kind == "postcommit-failure" ||
				kind == "bounded" || kind == "model-settings" {
				require.Equal(t, 1, strings.Count(request, "User plan edits take priority"))
				if kind == "coalesced" {
					require.Contains(t, request, "latest user save")
					require.NotContains(t, request, "first user save")
				}
			} else {
				require.NotContains(t, request, "User plan edits take priority")
			}
		})
	}
}
