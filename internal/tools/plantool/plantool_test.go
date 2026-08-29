package plantool_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/plantool"
)

func TestToolUpdatesCanonicalPlanFromStepsOnly(t *testing.T) {
	var gotItems []session.PlanItem
	plan := plantool.Tool(plantool.Deps{
		Update: func(_ context.Context, items []session.PlanItem) (session.Plan, error) {
			gotItems = items
			return session.Plan{Revision: 3, Approved: true, Items: items}, nil
		},
	})
	require.Equal(t, "plan", plan.Definition.Name)

	result, err := plan.Run(t.Context(), json.RawMessage(`{
		"steps":[
			{"content":"inspect","status":"completed","evidence":"targeted test passes"},
			{"content":"implement","status":"in_progress","note":"keep patch local"}
		]
	}`))
	require.NoError(t, err)
	require.Len(t, gotItems, 2)
	assert.Equal(t, session.PlanInProgress, gotItems[1].Status)
	assert.JSONEq(t, `{
		"revision":3,
		"approved":true,
		"items":[
			{"content":"inspect","status":"completed","evidence":"targeted test passes"},
			{"content":"implement","status":"in_progress","note":"keep patch local"}
		],
		"compatibility":"steps-only"
	}`, result.Content)
	assert.Equal(t, "update 2 steps", plan.DetailFromArgs(json.RawMessage(`{"steps":[{},{}]}`)))
}

func TestToolDefinitionAdvertisesDiscriminatedContract(t *testing.T) {
	definition := plantool.Tool(plantool.Deps{}).Definition
	require.NotNil(t, definition.Params)
	assert.Equal(t, []string{"action"}, definition.Params.Required)

	action, ok := definition.Params.Properties["action"].(map[string]any)
	require.True(t, ok, "action must be advertised")
	assert.Equal(t, []string{
		"create", "get", "patch", "update",
		"start", "complete", "block", "resume", "cancel", "reopen",
	}, action["enum"])

	view, ok := definition.Params.Properties["view"].(map[string]any)
	require.True(t, ok, "view must be advertised")
	assert.Equal(t, []string{"active", "full", "telemetry"}, view["enum"])

	for _, field := range []string{"goal", "approach", "successCriteria", "constraints", "workingContext"} {
		_, ok := definition.Params.Properties[field]
		assert.True(t, ok, "%s must be advertised", field)
	}

	_, ok = definition.Params.Properties["expected_revision"].(map[string]any)
	assert.True(t, ok, "expected_revision must be advertised for action patch")
	_, ok = definition.Params.Properties["ops"].(map[string]any)
	assert.True(t, ok, "ops must be advertised for action patch")
	for _, field := range []string{
		"id", "mutationId", "outcome", "evidence", "evidenceRefs",
		"noEvidenceReason", "blocker", "resumeWhen", "reason",
	} {
		_, ok := definition.Params.Properties[field]
		assert.True(t, ok, "%s must be advertised for the lifecycle actions", field)
	}
	assert.Contains(t, definition.Description, "compact projection")
}

func TestToolDefinitionUsesConfiguredRequiredStepTypes(t *testing.T) {
	definition := plantool.Tool(plantool.Deps{StepTypes: []string{"inspect", "change"}}).Definition
	raw, err := json.Marshal(definition.Params)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"type":"object",
		"properties":{
			"action":{
				"type":"string",
				"description":"Discriminates the call: create sends the full work contract, get reads the current plan, patch applies atomic ops against expected_revision, start/complete/block/resume/cancel/reopen move one step through the lifecycle, update replaces the ordered steps only (legacy).",
				"enum":["create","get","patch","update","start","complete","block","resume","cancel","reopen"]
			},
			"view":{
				"type":"string",
				"description":"Response shape for action get; default active. The telemetry view is the bounded observability snapshot: counters only, no plan content.",
				"enum":["active","full","telemetry"]
			},
			"expected_revision":{
				"type":"integer",
				"description":"Revision the patch expects; required for action patch. A stale value returns the actual revision."
			},
			"goal":{"type":"string","description":"One-sentence outcome the plan exists to reach; required for create.","maxLength":512},
			"approach":{"type":"string","description":"Chosen strategy in brief; required for create.","maxLength":1024},
			"successCriteria":{
				"type":"array",
				"description":"Observable conditions that prove the goal; at least one; required for create.",
				"maxItems":8,
				"items":{"type":"string","maxLength":512}
			},
			"constraints":{
				"type":"array",
				"description":"Hard limits the plan must respect.",
				"maxItems":8,
				"items":{"type":"string","maxLength":512}
			},
			"workingContext":{"type":"string","description":"Bounded context the steps assume.","maxLength":2048},
			"actions":{
				"type":"array",
				"maxItems":4,
				"description":"Built-in automations bound to the whole plan (create); patch ops set them via set_plan_fields; the harness runs them at the event, and a failed action rejects the transition.",
				"items":{
					"type":"object",
					"properties":{
						"event":{"type":"string","enum":["plan_start","plan_end"],"description":"Lifecycle moment the action fires on."},
						"type":{"type":"string","enum":["compact","inject_skill"],"description":"compact runs context compaction; inject_skill loads named skills before the step's first turn."},
						"skills":{"type":"array","maxItems":4,"description":"inject_skill: 1-4 skill names; compact carries none.","items":{"type":"string","maxLength":64}}
					},
					"required":["event","type"]
				}
			},
			"modelsByType":{
				"type":"object",
				"description":"Model per step type; a step's model override wins, unlisted types follow the session model.",
				"properties":{
					"inspect":{"type":"string","maxLength":128,"description":"Model for this step type."},
					"change":{"type":"string","maxLength":128,"description":"Model for this step type."}
				}
			},
			"steps":{
				"type":"array",
				"description":"Complete ordered plan snapshot; maximum 32 steps.",
				"maxItems":32,
				"items":{
					"type":"object",
					"properties":{
						"content":{"type":"string","description":"Specific actionable step; maximum 512 characters.","maxLength":512},
						"status":{"type":"string","enum":["pending","in_progress","blocked","completed","cancelled"]},
						"type":{"type":"string","description":"What this step is allowed to do.","enum":["inspect","change"]},
						"note":{"type":"string","description":"Optional concise finding, assumption, or blocker reason; maximum 512 characters.","maxLength":512},
						"evidence":{"type":"string","description":"Optional concise proof or verification result; maximum 512 characters.","maxLength":512},
						"id":{"type":"string","description":"Stable slug identifying this step; required for create.","maxLength":64},
						"why":{"type":"string","description":"Why this step exists; required for create.","maxLength":512},
						"doneWhen":{"type":"string","description":"Observable condition that ends this step; required for create.","maxLength":512},
						"risk":{"type":"string","description":"What could go wrong and the blast radius.","maxLength":512},
						"jit":{"type":"boolean","description":"True when the step is irreversible and needs just-in-time approval."},
						"model":{"type":"string","maxLength":128,"description":"Model override for this step; empty follows modelsByType for the step's type, then the session model."},
						"actions":{
							"type":"array",
							"maxItems":4,
							"description":"Built-in automations bound to this step; the harness runs them at the event, and a failed action rejects the transition.",
							"items":{
								"type":"object",
								"properties":{
									"event":{"type":"string","enum":["step_start","step_end"],"description":"Lifecycle moment the action fires on."},
									"type":{"type":"string","enum":["compact","inject_skill"],"description":"compact runs context compaction; inject_skill loads named skills before the step's first turn."},
									"skills":{"type":"array","maxItems":4,"description":"inject_skill: 1-4 skill names; compact carries none.","items":{"type":"string","maxLength":64}}
								},
								"required":["event","type"]
							}
						}
					},
					"required":["content","status","type"]
				}
			},
			"ops":{
				"type":"array",
				"description":"Atomic patch batch for action patch; maximum 32 ops, applied all-or-none against expected_revision. Each op reads only its own fields; scalar slots: absent keeps the value, a value replaces it, JSON null clears an optional one.",
				"maxItems":32,
				"items":{
					"type":"object",
					"properties":{
						"op":{
							"type":"string",
							"enum":[
								"set_plan_fields","replace_context","update_step","insert_step",
								"remove_step","reorder_steps",
								"add_constraint","update_constraint","remove_constraint",
								"add_criterion","update_criterion","remove_criterion"
							]
						},
						"goal":{"type":"string","maxLength":512,"description":"set_plan_fields."},
						"approach":{"type":"string","maxLength":1024,"description":"set_plan_fields."},
						"workingContext":{"type":"string","maxLength":2048,"description":"replace_context: the whole working context; null or empty clears it."},
						"modelsByType":{
							"type":"object",
							"description":"Model per step type; a step's model override wins, unlisted types follow the session model.",
							"properties":{
								"inspect":{"type":"string","maxLength":128,"description":"Model for this step type."},
								"change":{"type":"string","maxLength":128,"description":"Model for this step type."}
							}
						},
						"actions":{
							"type":"array",
							"maxItems":4,
							"description":"Built-in automations bound to update_step (step-level events) or set_plan_fields (plan-level events); replaces the list, null clears; the harness runs them at the event, and a failed action rejects the transition.",
							"items":{
								"type":"object",
								"properties":{
									"event":{"type":"string","enum":["step_start","step_end","plan_start","plan_end"],"description":"Lifecycle moment the action fires on."},
									"type":{"type":"string","enum":["compact","inject_skill"],"description":"compact runs context compaction; inject_skill loads named skills before the step's first turn."},
									"skills":{"type":"array","maxItems":4,"description":"inject_skill: 1-4 skill names; compact carries none.","items":{"type":"string","maxLength":64}}
								},
								"required":["event","type"]
							}
						},
						"model":{"type":"string","maxLength":128,"description":"update_step: model override for the step; empty follows the type map, null clears."},
						"id":{"type":"string","description":"update_step / remove_step target step id."},
						"content":{"type":"string","maxLength":512,"description":"update_step."},
						"why":{"type":"string","maxLength":512,"description":"update_step."},
						"doneWhen":{"type":"string","maxLength":512,"description":"update_step."},
						"risk":{"type":"string","maxLength":512,"description":"update_step; optional, null clears."},
						"note":{"type":"string","maxLength":512,"description":"update_step operational note; optional, null clears."},
						"before":{"type":"string","description":"insert_step anchor: place the new step before this id."},
						"after":{"type":"string","description":"insert_step anchor: place the new step after this id."},
						"step":{
							"type":"object",
							"description":"insert_step payload; starts pending.",
							"properties":{
								"id":{"type":"string","maxLength":64,"description":"Stable slug; required."},
								"content":{"type":"string","maxLength":512,"description":"Required."},
								"type":{"type":"string","enum":["inspect","change"],"description":"Required."},
								"why":{"type":"string","maxLength":512,"description":"Required."},
								"doneWhen":{"type":"string","maxLength":512,"description":"Required."},
								"risk":{"type":"string","maxLength":512},
								"jit":{"type":"boolean"},
								"model":{"type":"string","maxLength":128,"description":"Model override; empty follows the type map."},
								"actions":{
									"type":"array",
									"maxItems":4,
									"description":"Built-in automations bound to this step; the harness runs them at the event, and a failed action rejects the transition.",
									"items":{
										"type":"object",
										"properties":{
											"event":{"type":"string","enum":["step_start","step_end"],"description":"Lifecycle moment the action fires on."},
											"type":{"type":"string","enum":["compact","inject_skill"],"description":"compact runs context compaction; inject_skill loads named skills before the step's first turn."},
											"skills":{"type":"array","maxItems":4,"description":"inject_skill: 1-4 skill names; compact carries none.","items":{"type":"string","maxLength":64}}
										},
										"required":["event","type"]
									}
								}
							},
							"required":["id","content","type","why","doneWhen"]
						},
						"ids":{
							"type":"array",
							"maxItems":32,
							"description":"reorder_steps: the complete new order of every step id.",
							"items":{"type":"string","maxLength":64}
						},
						"value":{"type":"string","maxLength":512,"description":"add_/remove_ directive text (its identity)."},
						"from":{"type":"string","maxLength":512,"description":"update_ directive current text."},
						"to":{"type":"string","maxLength":512,"description":"update_ directive replacement text."}
					},
					"required":["op"]
				}
			},
			"id":{"type":"string","maxLength":64,"description":"Lifecycle target step id; required for start/complete/block/resume/cancel/reopen. Reopen without id addresses the closed plan itself."},
			"mutationId":{"type":"string","maxLength":64,"description":"Idempotency key for one lifecycle action; a retry with the same id replays the recorded result."},
			"outcome":{"type":"string","maxLength":512,"description":"complete: concise result the step produced; required."},
			"evidence":{"type":"string","maxLength":512,"description":"complete: concise proof; required unless evidence_refs or no_evidence_reason is sent."},
			"evidenceRefs":{"type":"array","maxItems":8,"description":"complete: bounded artifacts that prove the outcome; cite a recorded successful attempt as call:<its callId>.","items":{"type":"string","maxLength":128}},
			"noEvidenceReason":{"type":"string","maxLength":512,"description":"complete: why no evidence can exist; only valid without evidence."},
			"blocker":{"type":"string","maxLength":512,"description":"block: what blocks the step; required."},
			"resumeWhen":{"type":"string","maxLength":512,"description":"block: the condition that unblocks the step; required."},
			"reason":{"type":"string","maxLength":512,"description":"cancel / reopen: why; required."},
			"planResult":{"type":"string","enum":["success","abandoned"],"description":"complete: close the whole plan in the same write when this step is the last active work; success asserts the success criteria are met. Refused while any step is pending, in_progress or blocked, or (for success) when a step was cancelled."}
		},
		"required":["action"]
	}`, string(raw))
}

// TestTransitionReceiptReportsPlanClosed: a completing transition that also
// closes the plan answers with the close riding the receipt, and a reopen
// without a step id describes itself as addressing the plan.
func TestTransitionReceiptReportsPlanClosed(t *testing.T) {
	var got session.PlanTransition
	tool := plantool.Tool(plantool.Deps{
		Transition: func(_ context.Context, tr session.PlanTransition) (session.Plan, session.PlanTransitionResult, error) {
			got = tr
			return session.Plan{Approved: true, Result: session.PlanResultAbandoned},
				session.PlanTransitionResult{
					Action: session.TransitionComplete, StepID: "alpha",
					From: session.PlanInProgress, To: session.PlanCompleted, Revision: 5,
					PlanClosed: session.PlanResultAbandoned,
				}, nil
		},
	})

	result, err := tool.Run(t.Context(), json.RawMessage(`{
		"action":"complete","id":"alpha","mutationId":"close-1",
		"outcome":"alpha concluded","evidence":"focused tests",
		"planResult":"abandoned"
	}`))
	require.NoError(t, err)
	assert.Equal(t, session.PlanResultAbandoned, got.PlanResult, "planResult rides the transition payload")
	assert.Contains(t, result.Content, `"planClosed":"abandoned"`)
	assert.Contains(t, result.Detail, "plan closed (abandoned)")

	assert.Equal(
		t, "reopen plan",
		tool.DetailFromArgs(json.RawMessage(`{"action":"reopen","reason":"scope moved"}`)),
	)
}

func TestToolToleratesLegacyUpdateMetadata(t *testing.T) {
	calls := 0
	plan := plantool.Tool(plantool.Deps{
		Update: func(_ context.Context, items []session.PlanItem) (session.Plan, error) {
			calls++
			return session.Plan{Revision: 8, Items: items}, nil
		},
	})

	_, err := plan.Run(t.Context(), json.RawMessage(
		`{"action":"update","expected_revision":7,"steps":[]}`,
	))
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestToolValidatesStepsOnlyInput(t *testing.T) {
	calls := 0
	plan := plantool.Tool(plantool.Deps{
		Update: func(_ context.Context, items []session.PlanItem) (session.Plan, error) {
			calls++
			require.NotNil(t, items)
			return session.Plan{Revision: 1, Items: items}, nil
		},
	})

	for _, args := range []string{
		`{}`,
		`{"steps":null}`,
		`{"steps":[],"extra":true}`,
		`{"action":"replace","steps":[]}`,
	} {
		_, err := plan.Run(t.Context(), json.RawMessage(args))
		require.Error(t, err, args)
	}
	assert.Zero(t, calls)

	_, err := plan.Run(t.Context(), json.RawMessage(`{"steps":[]}`))
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

// Attempts are harness-recorded evidence: no authoring action accepts them,
// so a contract that arrives carrying them is refused, not silently stripped.
func TestToolRefusesStepsWithAttempts(t *testing.T) {
	calls := 0
	plan := plantool.Tool(plantool.Deps{
		Create: func(_ context.Context, _ session.PlanV2) (session.Plan, []session.PlanMaterialChange, error) {
			calls++
			return session.Plan{}, nil, nil
		},
		Update: func(_ context.Context, _ []session.PlanItem) (session.Plan, error) {
			calls++
			return session.Plan{}, nil
		},
	})

	// create names the offense; update folds it into its steps-only refusal.
	_, err := plan.Run(t.Context(), json.RawMessage(
		`{"action":"create","goal":"g","approach":"a","successCriteria":["c"],
		"steps":[{"id":"s","content":"c","status":"pending","type":"edit","why":"w","doneWhen":"d",
		"attempts":[{"callId":"fake","tool":"read","status":"success"}]}]}`,
	))
	require.ErrorContains(t, err, "steps take no attempts")

	_, err = plan.Run(t.Context(), json.RawMessage(
		`{"action":"update","steps":[{"content":"c","status":"in_progress","type":"edit",
		"attempts":[{"callId":"fake","tool":"read","status":"success"}]}]}`,
	))
	require.ErrorContains(t, err, "plan update is steps-only")
	assert.Zero(t, calls, "no authoring path may reach the session with attempts")
}
