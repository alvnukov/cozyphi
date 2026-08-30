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

// skillsSeamDeps records what crossed the tool seam; every action succeeds so
// the tests observe the pass-through contract, not session validation.
func skillsSeamDeps(created *session.PlanV2, ops *[]session.PlanPatchOp) plantool.Deps {
	return plantool.Deps{
		Create: func(_ context.Context, contract session.PlanV2) (session.Plan, []session.PlanMaterialChange, error) {
			*created = contract
			return session.Plan{Revision: 1, Schema: session.PlanSchemaV2, Items: contract.Items}, nil, nil
		},
		Patch: func(_ context.Context, _ uint64, batch []session.PlanPatchOp) (session.Plan, session.PlanPatchSummary, error) {
			*ops = batch
			return session.Plan{Revision: 2, Schema: session.PlanSchemaV2}, session.PlanPatchSummary{}, nil
		},
		Get: func(context.Context) (session.Plan, error) {
			return session.Plan{Revision: 1, Schema: session.PlanSchemaV2}, nil
		},
	}
}

// TestToolSkillsRideCreateAndPatchOps: skills is the one automation field the
// model owns. create steps and insert_step/supersede_step step objects carry
// the wire list into the contract untouched (the session compiles it), and
// update_step.skills rides the op with set semantics.
func TestToolSkillsRideCreateAndPatchOps(t *testing.T) {
	var created session.PlanV2
	var ops []session.PlanPatchOp
	tool := plantool.Tool(skillsSeamDeps(&created, &ops))

	_, err := tool.Run(t.Context(), json.RawMessage(`{
		"action":"create","goal":"g","approach":"a","successCriteria":["c"],
		"steps":[{"id":"s","content":"c","type":"edit","why":"w","doneWhen":"d","skills":["tdd","code-review"]}]}`))
	require.NoError(t, err)
	assert.Equal(t, []string{"tdd", "code-review"}, created.Items[0].Skills,
		"the wire list rides the contract; the session compiles it")

	_, err = tool.Run(t.Context(), json.RawMessage(`{
		"action":"patch","ops":[
			{"op":"update_step","id":"s","skills":["tdd"]},
			{"op":"insert_step","after":"s","step":{"id":"n","content":"c","type":"edit","why":"w","doneWhen":"d","skills":["grill"]}},
			{"op":"supersede_step","id":"n","step":{"id":"n2","content":"c","type":"edit","why":"w","doneWhen":"d","skills":[]}}]}`))
	require.NoError(t, err)
	require.Len(t, ops, 3)
	assert.True(t, ops[0].Skills.Set, "update_step.skills crosses the seam as a set slot")
	assert.Equal(t, []string{"tdd"}, ops[0].Skills.Value)
	assert.Equal(t, []string{"grill"}, ops[1].Step.Skills)
	assert.NotNil(t, ops[2].Step.Skills, "an explicit empty list is authorship and must survive the seam")
}

// TestToolValidatesSkillsAgainstCatalog: names the skill catalog does not know
// fail closed at the tool seam — create, update_step, and insert_step alike —
// while catalog names pass.
func TestToolValidatesSkillsAgainstCatalog(t *testing.T) {
	var created session.PlanV2
	var ops []session.PlanPatchOp
	deps := skillsSeamDeps(&created, &ops)
	deps.Skills = func() []string { return []string{"tdd", "grill"} }
	tool := plantool.Tool(deps)

	for name, args := range map[string]string{
		"create step skills": `{"action":"create","goal":"g","approach":"a","successCriteria":["c"],
			"steps":[{"id":"s","content":"c","type":"edit","why":"w","doneWhen":"d","skills":["ghost"]}]}`,
		"update_step skills": `{"action":"patch","ops":[{"op":"update_step","id":"s","skills":["tdd","ghost"]}]}`,
		"insert_step skills": `{"action":"patch","ops":[{"op":"insert_step","step":{"id":"n","content":"c","type":"edit","why":"w","doneWhen":"d","skills":["ghost"]}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := tool.Run(t.Context(), json.RawMessage(args))
			require.ErrorContains(t, err, `unknown skill "ghost"`)
		})
	}

	_, err := tool.Run(t.Context(), json.RawMessage(`{
		"action":"create","goal":"g","approach":"a","successCriteria":["c"],
		"steps":[{"id":"s","content":"c","type":"edit","why":"w","doneWhen":"d","skills":["tdd"]}]}`))
	require.NoError(t, err, "catalog names pass")
}

// TestToolLegacyUpdateRefusesStepSkills: skills is a v2 affordance; the legacy
// steps-only replace refuses it instead of silently dropping the authoring.
func TestToolLegacyUpdateRefusesStepSkills(t *testing.T) {
	tool := plantool.Tool(plantool.Deps{
		Update: func(context.Context, []session.PlanItem) (session.Plan, error) {
			t.Fatal("legacy update must not run for skills-carrying steps")
			return session.Plan{}, nil
		},
	})
	_, err := tool.Run(t.Context(), json.RawMessage(
		`{"action":"update","steps":[{"content":"c","status":"pending","skills":["tdd"]}]}`,
	))
	require.ErrorContains(t, err, "steps-only")
}

// TestToolDefinitionAdvertisesSkills: the schema names the model-owned surface
// on step objects (create, insert_step, supersede_step) and on update_step.
func TestToolDefinitionAdvertisesSkills(t *testing.T) {
	definition := plantool.Tool(plantool.Deps{}).Definition
	raw, err := json.Marshal(definition.Params)
	require.NoError(t, err)
	var params struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(raw, &params))

	var steps struct {
		Items struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(params.Properties["steps"], &steps))
	_, advertised := steps.Items.Properties["skills"]
	assert.True(t, advertised, "steps[].skills must be advertised")

	var ops struct {
		Items struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(params.Properties["ops"], &ops))
	_, advertised = ops.Items.Properties["skills"]
	assert.True(t, advertised, "ops[].skills must be advertised for update_step")

	var stepObject struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(ops.Items.Properties["step"], &stepObject))
	_, advertised = stepObject.Properties["skills"]
	assert.True(t, advertised, "ops[].step.skills must be advertised for insert_step")
}
