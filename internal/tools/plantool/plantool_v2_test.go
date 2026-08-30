package plantool_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/plantool"
)

// errGoalRequired mirrors the session layer's required-field error so the
// wrapping test exercises the real failure shape.
var errGoalRequired = errors.New("session: plan goal is required")

// v2PlanFixture is one durable v2 snapshot with every view-relevant shape:
// done, in-progress, blocked, and pending steps plus contract fields.
func v2PlanFixture() session.Plan {
	return session.Plan{
		Revision:        9,
		Approved:        true,
		Schema:          session.PlanSchemaV2,
		Goal:            "ship the plan v2 tool contract",
		Approach:        "adapter over the canonical session model",
		SuccessCriteria: []string{"compact get stays bounded", "full get is canonical"},
		Constraints:     []string{"no schema drift", "gate untouched"},
		WorkingContext:  "worktree plan-v2-create-get-actions",
		// Human-owned settings the fixture carries so the views can pin that
		// they never cross into a model-facing answer.
		ModelsByType: map[session.StepType]string{session.StepEdit: "opus"},
		Actions: []session.PlanAction{{
			Event: session.PlanActionOnPlanStart,
			Type:  session.PlanActionCompact,
		}},
		Items: []session.PlanItem{
			{
				ID:       "audit",
				Content:  "audit the legacy tool",
				Status:   session.PlanCompleted,
				Type:     session.StepExplore,
				Evidence: "read the seam",
			},
			{
				ID:       "wire-tool",
				Content:  "wire the tool actions",
				Status:   session.PlanInProgress,
				Type:     session.StepEdit,
				DoneWhen: "contract tests pass",
				Model:    "haiku",
				Actions: []session.PlanAction{{
					Event:  session.PlanActionOnStepStart,
					Type:   session.PlanActionInjectSkill,
					Skills: []string{"tdd"},
				}},
			},
			{
				ID:      "wait-review",
				Content: "await field review",
				Status:  session.PlanBlocked,
				Type:    session.StepDelegate,
				Note:    "reviewer offline",
			},
			{
				ID:       "migrate-callers",
				Content:  "migrate callers",
				Status:   session.PlanPending,
				Type:     session.StepEdit,
				DoneWhen: "suite green",
			},
		},
	}
}

const createArgs = `{
	"action":"create",
	"goal":"ship the plan v2 tool contract",
	"approach":"adapter over the canonical session model",
	"successCriteria":["compact get stays bounded"],
	"constraints":["no schema drift"],
	"workingContext":"worktree plan-v2-create-get-actions",
	"steps":[
		{"id":"wire-tool","content":"wire the tool actions","status":"completed","type":"edit","why":"tool is the seam","doneWhen":"contract tests pass","risk":"schema drift","jit":true,"evidence":"tests green"},
		{"id":"migrate-callers","content":"migrate callers","status":"pending","type":"edit","why":"callers follow the seam","doneWhen":"suite green"}
	]
}`

func TestToolCreatesUnapprovedV2Draft(t *testing.T) {
	var gotContract session.PlanV2
	updates := 0
	plan := plantool.Tool(plantool.Deps{
		Update: func(context.Context, []session.PlanItem) (session.Plan, error) {
			updates++
			return session.Plan{}, nil
		},
		Create: func(_ context.Context, contract session.PlanV2) (session.Plan, []session.PlanMaterialChange, error) {
			gotContract = contract
			return session.Plan{
				Revision: 5,
				Schema:   session.PlanSchemaV2,
				Goal:     contract.Goal,
				Items:    contract.Items,
			}, nil, nil
		},
	})

	result, err := plan.Run(t.Context(), json.RawMessage(createArgs))
	require.NoError(t, err)
	assert.Zero(t, updates, "create must not touch the legacy update path")

	assert.Equal(t, "ship the plan v2 tool contract", gotContract.Goal)
	assert.Equal(t, "adapter over the canonical session model", gotContract.Approach)
	assert.Equal(t, []string{"compact get stays bounded"}, gotContract.SuccessCriteria)
	assert.Equal(t, []string{"no schema drift"}, gotContract.Constraints)
	assert.Equal(t, "worktree plan-v2-create-get-actions", gotContract.WorkingContext)
	require.Len(t, gotContract.Items, 2)
	assert.Equal(t, "wire-tool", gotContract.Items[0].ID)
	assert.Equal(t, "tool is the seam", gotContract.Items[0].Why)
	assert.Equal(t, "contract tests pass", gotContract.Items[0].DoneWhen)
	assert.Equal(t, "schema drift", gotContract.Items[0].Risk)
	assert.True(t, gotContract.Items[0].JIT)
	assert.Equal(t, "tests green", gotContract.Items[0].Evidence)

	assert.JSONEq(t, `{
		"action":"create",
		"revision":5,
		"approved":false,
		"steps":{"total":2,"remaining":1}
	}`, result.Content)
	assert.Equal(t, "create 2 steps", plan.DetailFromArgs(json.RawMessage(createArgs)))
}

func TestToolActionsIgnoreProviderMaterializedForeignDefaults(t *testing.T) {
	var created session.PlanV2
	var updated []session.PlanItem
	gets := 0
	tool := plantool.Tool(plantool.Deps{
		Create: func(_ context.Context, contract session.PlanV2) (session.Plan, []session.PlanMaterialChange, error) {
			created = contract
			return session.Plan{Revision: 1, Schema: session.PlanSchemaV2, Items: contract.Items}, nil, nil
		},
		Update: func(_ context.Context, items []session.PlanItem) (session.Plan, error) {
			updated = items
			return session.Plan{Revision: 2, Items: items}, nil
		},
		Get: func(context.Context) (session.Plan, error) {
			gets++
			return session.Plan{}, nil
		},
	})

	defaults := `
		"view":"active",
		"expected_revision":0,
		"ops":[],
		"id":"",
		"mutationId":"",
		"outcome":"",
		"evidence":"",
		"evidenceRefs":[],
		"noEvidenceReason":"",
		"blocker":"",
		"resumeWhen":"",
		"reason":"",
		"planResult":"success"`

	_, err := tool.Run(t.Context(), json.RawMessage(`{
		"action":"create",
		"goal":"g",
		"approach":"a",
		"successCriteria":["c"],
		"constraints":[],
		"workingContext":"",
		"steps":[{"id":"s","content":"c","status":"pending","type":"explore","why":"w","doneWhen":"d"}],
		`+defaults+`
	}`))
	require.NoError(t, err)
	require.Len(t, created.Items, 1)

	_, err = tool.Run(t.Context(), json.RawMessage(`{
		"action":"update",
		"goal":"",
		"approach":"",
		"successCriteria":[],
		"constraints":[],
		"workingContext":"",
		"steps":[{"content":"new step","status":"pending","type":"explore","id":"","why":"","doneWhen":"","risk":"","jit":false}],
		`+defaults+`
	}`))
	require.NoError(t, err)
	require.Len(t, updated, 1)

	_, err = tool.Run(t.Context(), json.RawMessage(`{
		"action":"get",
		"goal":"provider default",
		"approach":"provider default",
		"successCriteria":[],
		"constraints":[],
		"workingContext":"",
		"steps":[],
		`+defaults+`
	}`))
	require.NoError(t, err)
	assert.Equal(t, 1, gets)
}

func TestToolGetActiveReturnsBoundedView(t *testing.T) {
	tool := plantool.Tool(plantool.Deps{
		Get: func(context.Context) (session.Plan, error) { return v2PlanFixture(), nil },
	})

	result, err := tool.Run(t.Context(), json.RawMessage(`{"action":"get"}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"action":"get",
		"view":"active",
		"revision":9,
		"approved":true,
		"progress":{"total":4,"done":1,"active":1,"blocked":1,"pending":1},
		"goal":"ship the plan v2 tool contract",
		"approach":"adapter over the canonical session model",
		"successCriteria":["compact get stays bounded","full get is canonical"],
		"constraints":["no schema drift","gate untouched"],
		"workingContext":"worktree plan-v2-create-get-actions",
		"active":{
			"id":"wire-tool",
			"content":"wire the tool actions",
			"status":"in_progress",
			"type":"edit",
			"doneWhen":"contract tests pass",
			"skills":[{"name":"tdd"}]
		},
		"blocked":[{
			"id":"wait-review",
			"content":"await field review",
			"status":"blocked",
			"type":"delegate",
			"note":"reviewer offline"
		}],
		"completed":[{"id":"audit","status":"completed"}],
		"next":[{"id":"migrate-callers","content":"migrate callers","status":"pending","type":"edit"}]
	}`, result.Content)
	assert.Equal(t, "get active", tool.DetailFromArgs(json.RawMessage(`{"action":"get"}`)))

	// The compact view must stay cheaper than the canonical snapshot it hides.
	full, err := tool.Run(t.Context(), json.RawMessage(`{"action":"get","view":"full"}`))
	require.NoError(t, err)
	assert.Less(t, len(result.Content), len(full.Content))
}

func TestToolGetFullReturnsCanonicalSnapshotMinusHumanOnlyFields(t *testing.T) {
	fixture := v2PlanFixture()
	tool := plantool.Tool(plantool.Deps{
		Get: func(context.Context) (session.Plan, error) { return fixture, nil },
	})

	result, err := tool.Run(t.Context(), json.RawMessage(`{"action":"get","view":"full"}`))
	require.NoError(t, err)

	// The canonical shape minus the human-owned settings: the user's TUI
	// renders the real snapshot, the model never reads pins or automation.
	// Skills stay — the model authored them.
	want := fixture
	want.Actions = nil
	want.ModelsByType = nil
	want.Items = append([]session.PlanItem(nil), fixture.Items...)
	for i := range want.Items {
		want.Items[i].Model = ""
		want.Items[i].Actions = nil
	}
	// wire-tool carries the one inject_skill action; runs drop, the skills list
	// stays, every other action type is human configuration and disappears.
	want.Items[1].Actions = []session.PlanAction{{
		Event:  session.PlanActionOnStepStart,
		Type:   session.PlanActionInjectSkill,
		Skills: []string{"tdd"},
	}}
	encoded, err := json.Marshal(want)
	require.NoError(t, err)
	assert.Equal(t, string(encoded), result.Content, "full view is canonical minus model pins and human actions")
	assert.NotContains(t, result.Content, `"model"`, "step model pins never reach the model")
	assert.Contains(t, result.Content, `"inject_skill"`, "the model's skill lists stay visible")
	assert.NotContains(t, result.Content, `"modelsByType"`, "the type map never reaches the model")
	assert.Equal(t, "get full", tool.DetailFromArgs(json.RawMessage(`{"action":"get","view":"full"}`)))
}

func TestToolGetDefaultsToActiveView(t *testing.T) {
	tool := plantool.Tool(plantool.Deps{
		Get: func(context.Context) (session.Plan, error) { return v2PlanFixture(), nil },
	})

	implicit, err := tool.Run(t.Context(), json.RawMessage(`{"action":"get"}`))
	require.NoError(t, err)
	explicit, err := tool.Run(t.Context(), json.RawMessage(`{"action":"get","view":"active"}`))
	require.NoError(t, err)
	assert.JSONEq(t, explicit.Content, implicit.Content)
}

func TestToolGetServesLegacyPlansCompactly(t *testing.T) {
	tool := plantool.Tool(plantool.Deps{
		Get: func(context.Context) (session.Plan, error) {
			return session.Plan{
				Revision: 4,
				Items: []session.PlanItem{
					{Content: "inspect the seam", Status: session.PlanCompleted, Type: session.StepExplore},
					{
						Content: "widen the contract",
						Status:  session.PlanInProgress,
						Type:    session.StepEdit,
						Note:    "one truth",
					},
				},
			}, nil
		},
	})

	result, err := tool.Run(t.Context(), json.RawMessage(`{"action":"get"}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"action":"get",
		"view":"active",
		"revision":4,
		"approved":false,
		"progress":{"total":2,"done":1,"active":1},
		"active":{
			"content":"widen the contract",
			"status":"in_progress",
			"type":"edit",
			"note":"one truth"
		},
		"completed":[{"content":"inspect the seam","status":"completed"}]
	}`, result.Content)
}

func TestToolGetActiveHandlesEmptyPlan(t *testing.T) {
	tool := plantool.Tool(plantool.Deps{
		Get: func(context.Context) (session.Plan, error) { return session.Plan{}, nil },
	})

	result, err := tool.Run(t.Context(), json.RawMessage(`{"action":"get"}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"action":"get","view":"active","revision":0,"approved":false}`, result.Content)
}

func TestToolRejectsUnknownActionsAndInvalidSelectedPayload(t *testing.T) {
	updates, creates, gets := 0, 0, 0
	tool := plantool.Tool(plantool.Deps{
		Update: func(context.Context, []session.PlanItem) (session.Plan, error) {
			updates++
			return session.Plan{Revision: 1}, nil
		},
		Create: func(_ context.Context, contract session.PlanV2) (session.Plan, []session.PlanMaterialChange, error) {
			creates++
			// The session layer owns the required-field texts; the tool wraps.
			if contract.Goal == "" {
				return session.Plan{}, nil, errGoalRequired
			}
			return session.Plan{Revision: 2, Items: contract.Items}, nil, nil
		},
		Get: func(context.Context) (session.Plan, error) {
			gets++
			return session.Plan{}, nil
		},
	})

	for _, tc := range []struct {
		name        string
		args        string
		wantErr     string
		wantUpdates int
		wantCreates int
		wantGets    int
	}{
		{
			name:        "unknown action names the allowed set",
			args:        `{"action":"replace","steps":[]}`,
			wantErr:     `unsupported action "replace"`,
			wantUpdates: 0,
		},
		{
			name:     "unknown view names the allowed views",
			args:     `{"action":"get","view":"everything"}`,
			wantErr:  `unsupported view "everything"`,
			wantGets: 0,
		},
		{
			name:    "create without steps",
			args:    `{"action":"create","goal":"g","approach":"a","successCriteria":["c"]}`,
			wantErr: "steps is required",
		},
		{
			name:        "incomplete contract surfaces the session error",
			args:        `{"action":"create","approach":"a","successCriteria":["c"],"steps":[{"id":"s","content":"c","status":"pending","type":"edit","why":"w","doneWhen":"d"}]}`,
			wantErr:     "plan create: session: plan goal is required",
			wantCreates: 1,
		},
		{
			name:    "update cannot carry step v2 fields",
			args:    `{"action":"update","steps":[{"content":"c","status":"pending","id":"s1"}]}`,
			wantErr: "steps-only",
		},
		{
			name:    "create rejects unknown fields",
			args:    `{"action":"create","steps":[],"nope":1}`,
			wantErr: "unknown field",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			beforeUpdates, beforeCreates, beforeGets := updates, creates, gets
			_, err := tool.Run(t.Context(), json.RawMessage(tc.args))
			require.ErrorContains(t, err, tc.wantErr)
			assert.Equal(t, beforeUpdates+tc.wantUpdates, updates, tc.name)
			assert.Equal(t, beforeCreates+tc.wantCreates, creates, tc.name)
			assert.Equal(t, beforeGets+tc.wantGets, gets, tc.name)
		})
	}
}

func TestToolCompactViewStaysWellUnderFullSnapshot(t *testing.T) {
	// A maximal plan: 32 steps with every prose field at its durable cap, plus
	// full-length approach and working context. The compact view must shed the
	// bulk of it whatever the plan grows to.
	prose := strings.Repeat("x", 512)
	plan := v2PlanFixture()
	plan.Approach = strings.Repeat("a", 1024)
	plan.WorkingContext = strings.Repeat("w", 2048)
	for len(plan.Items) < 32 {
		plan.Items = append(plan.Items, session.PlanItem{
			ID:       fmt.Sprintf("step-%d", len(plan.Items)+1),
			Content:  prose,
			Status:   session.PlanPending,
			Type:     session.StepEdit,
			Why:      prose,
			DoneWhen: prose,
			Risk:     prose,
			Note:     prose,
			Evidence: prose,
		})
	}
	tool := plantool.Tool(plantool.Deps{
		Get: func(context.Context) (session.Plan, error) { return plan, nil },
	})

	compact, err := tool.Run(t.Context(), json.RawMessage(`{"action":"get"}`))
	require.NoError(t, err)
	full, err := tool.Run(t.Context(), json.RawMessage(`{"action":"get","view":"full"}`))
	require.NoError(t, err)
	assert.Less(t, len(compact.Content), len(full.Content)/4,
		"the compact view must stay a fraction of a maximal canonical snapshot")
}

func TestToolLegacyUpdateCarriesCompatibilityMarker(t *testing.T) {
	tool := plantool.Tool(plantool.Deps{
		Update: func(_ context.Context, items []session.PlanItem) (session.Plan, error) {
			return session.Plan{Revision: 8, Items: items}, nil
		},
	})

	result, err := tool.Run(t.Context(), json.RawMessage(
		`{"action":"update","expected_revision":7,"steps":[{"content":"inspect","status":"completed"}]}`,
	))
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"revision":8,
		"approved":false,
		"items":[{"content":"inspect","status":"completed"}],
		"compatibility":"steps-only"
	}`, result.Content)

	// A bare steps-only call — the pre-v2 wire shape — stays on the same path.
	bare, err := tool.Run(t.Context(), json.RawMessage(`{"steps":[]}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"revision":8,"approved":false,"items":[],"compatibility":"steps-only"}`, bare.Content)
}
