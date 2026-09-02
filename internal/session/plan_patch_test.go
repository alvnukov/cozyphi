package session

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pv builds a set patch slot; pv("") is an explicit clear (JSON null).
func pv(s string) PatchValue[string] {
	return PatchValue[string]{Set: true, Value: s}
}

// patchedFixture returns a manager holding the approved v2 fixture at
// revision 2, so every test starts from one known durable state.
func patchedFixture(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, _, err = m.ReplacePlanV2(v2Fixture(), false)
	require.NoError(t, err)
	_, err = m.SetPlanApproved(true)
	require.NoError(t, err)
	require.Equal(t, uint64(2), m.Plan().Revision)
	require.True(t, m.Plan().Approved)
	return m
}

func TestPatchValueDistinguishesNullFromAbsent(t *testing.T) {
	var op PlanPatchOp
	require.NoError(t, json.Unmarshal([]byte(`{
		"op": "update_step",
		"id": "decode-legacy",
		"note": null,
		"risk": "still risky",
		"content": "rewritten"
	}`), &op))
	assert.True(t, op.Note.Set, "explicit null marks the slot as cleared")
	assert.Empty(t, op.Note.Value)
	assert.True(t, op.Risk.Set)
	assert.Equal(t, "still risky", op.Risk.Value)
	assert.True(t, op.Content.Set)
	assert.False(t, op.Why.Set, "an absent field leaves the slot untouched")
	assert.False(t, op.Goal.Set)
}

func TestPatchPlanAppliesAtomicBatch(t *testing.T) {
	m := patchedFixture(t)

	patched, summary, err := m.PatchPlan(2, []PlanPatchOp{
		{Op: PlanPatchSetPlanFields, Goal: pv("ship the plan v2 patch operations")},
		{Op: PlanPatchUpdateStep, ID: "decode-legacy", Note: pv("still decoding")},
		{
			Op:    PlanPatchInsertStep,
			After: "decode-legacy",
			Step: &PlanItem{
				ID:       "wire-patch",
				Content:  "wire patch into the plan tool",
				Type:     StepEdit,
				Why:      "models need an in-place editor",
				DoneWhen: "patch round-trips durably",
			},
		},
		{Op: PlanPatchAddConstraint, Value: "atomic all-or-none"},
	}, false)
	require.NoError(t, err)

	assert.Equal(t, uint64(3), patched.Revision)
	assert.Equal(t, "ship the plan v2 patch operations", patched.Goal)
	assert.False(t, patched.Approved, "a contract change resets approval")
	require.Len(t, patched.Items, 3)
	assert.Equal(t, "wire-patch", patched.Items[2].ID)
	assert.Equal(t, PlanPending, patched.Items[2].Status, "an inserted step starts pending")
	assert.Equal(t, "still decoding", patched.Items[1].Note)
	assert.Equal(t, []string{"no tool API switch in the expand phase", "atomic all-or-none"}, patched.Constraints)

	assert.Equal(t, PlanPatchSummary{
		PlanFields:    []string{"goal", "constraints"},
		StepsUpdated:  []string{"decode-legacy"},
		StepsInserted: []string{"wire-patch"},
		Diff: []PlanMaterialChange{
			{Target: "plan", Field: "goal", Change: MaterialChanged},
			{Target: "plan", Field: "constraints", Change: MaterialAdded, Detail: "atomic all-or-none"},
			{Target: "wire-patch", Field: "step", Change: MaterialAdded},
		},
	}, summary)

	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	// The durable snapshot is JSON, so canonicalize the in-memory plan
	// through the same round-trip before comparing; see roundPlanTimes.
	roundPlanTimes(t, &patched)
	assert.Equal(t, patched, loaded.Plan(), "the whole batch lands as one durable snapshot")
}

func TestPatchPlanRollsBackWholeBatchOnAnyOpError(t *testing.T) {
	m := patchedFixture(t)
	before := m.Plan()

	_, _, err := m.PatchPlan(2, []PlanPatchOp{
		{Op: PlanPatchUpdateStep, ID: "decode-legacy", Note: pv("must not survive")},
		{Op: PlanPatchRemoveStep, ID: "decode-legacy"},
	}, false)
	require.ErrorContains(
		t,
		err,
		`patch op 2 (remove_step): step "decode-legacy" is in_progress; only pending steps can be removed`,
	)

	after := m.Plan()
	roundPlanTimes(t, &before)
	roundPlanTimes(t, &after)
	assert.Equal(t, before, after, "a failing operation leaves no partial change behind")
	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	assert.Equal(t, before, loaded.Plan())
}

func TestPatchPlanRejectsStaleRevision(t *testing.T) {
	m := patchedFixture(t)
	before := m.Plan()

	_, _, err := m.PatchPlan(99, []PlanPatchOp{
		{Op: PlanPatchReplaceContext, WorkingContext: pv("stale write")},
	}, false)
	require.ErrorContains(t, err, "session: plan revision is 2; patch expected 99")

	// The refusal is typed so an editor holding a stale draft can rebase it
	// instead of parsing the sentence written for the model.
	var stale *StalePlanRevisionError
	require.ErrorAs(t, err, &stale)
	assert.Equal(t, uint64(99), stale.Expected)
	assert.Equal(t, uint64(2), stale.Actual)

	assert.Equal(t, before, m.Plan())
}

func TestPatchPlanRequiresV2Plan(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(false))
	require.NoError(t, err)
	_, err = m.ReplacePlan([]PlanItem{{Content: "legacy step", Status: PlanPending, Type: StepExplore}})
	require.NoError(t, err)

	_, _, err = m.PatchPlan(1, []PlanPatchOp{{Op: PlanPatchUpdateStep, ID: "legacy-step", Note: pv("no ids")}}, false)
	require.ErrorContains(t, err, "session: plan patch requires a v2 plan")
}

func TestPatchPlanScalarClearSemantics(t *testing.T) {
	m := patchedFixture(t)

	patched, _, err := m.PatchPlan(2, []PlanPatchOp{
		{Op: PlanPatchUpdateStep, ID: "decode-legacy", Risk: pv(""), Note: pv("")},
		{Op: PlanPatchReplaceContext, WorkingContext: pv("")},
	}, false)
	require.NoError(t, err)
	assert.Empty(t, patched.Items[1].Risk, "explicit null clears an optional field")
	assert.Empty(t, patched.Items[1].Note)
	assert.Empty(t, patched.WorkingContext)
	assert.Equal(t, "old sessions must resume without data loss", patched.Items[1].Why, "absent stays unchanged")

	revBeforeRejects := m.Plan().Revision
	for _, tc := range []struct {
		op   PlanPatchOp
		want string
	}{
		{op: PlanPatchOp{Op: PlanPatchSetPlanFields, Goal: pv("")}, want: "goal cannot be cleared; it is required"},
		{op: PlanPatchOp{Op: PlanPatchSetPlanFields, Approach: pv(" ")}, want: "approach cannot be cleared; it is required"},
		{
			op:   PlanPatchOp{Op: PlanPatchUpdateStep, ID: "decode-legacy", Content: pv("")},
			want: "content cannot be cleared; it is required",
		},
		{
			op:   PlanPatchOp{Op: PlanPatchUpdateStep, ID: "decode-legacy", Why: pv(" ")},
			want: "why cannot be cleared; it is required",
		},
		{
			op:   PlanPatchOp{Op: PlanPatchUpdateStep, ID: "decode-legacy", DoneWhen: pv("")},
			want: "done_when cannot be cleared; it is required",
		},
	} {
		_, _, err := m.PatchPlan(m.Plan().Revision, []PlanPatchOp{tc.op}, false)
		require.ErrorContains(t, err, tc.want)
		require.ErrorContains(t, err, "patch op 1 ("+tc.op.Op+")")
	}
	assert.Equal(t, revBeforeRejects, m.Plan().Revision, "rejected clears leave the revision alone")
}

func TestPatchPlanStepStructureRules(t *testing.T) {
	t.Run("remove only pending steps", func(t *testing.T) {
		m := patchedFixture(t)
		_, _, err := m.PatchPlan(2, []PlanPatchOp{
			{
				Op:    PlanPatchInsertStep,
				After: "decode-legacy",
				Step:  &PlanItem{ID: "temp", Content: "scratch", Type: StepExplore, Why: "w", DoneWhen: "d"},
			},
			{Op: PlanPatchRemoveStep, ID: "temp"},
		}, false)
		require.NoError(t, err)
		require.Len(t, m.Plan().Items, 2)

		_, _, err = m.PatchPlan(3, []PlanPatchOp{{Op: PlanPatchRemoveStep, ID: "write-schema"}}, false)
		require.ErrorContains(t, err, `step "write-schema" is completed; only pending steps can be removed`)
	})

	t.Run("insert anchors", func(t *testing.T) {
		m := patchedFixture(t)
		step := &PlanItem{ID: "fresh", Content: "c", Type: StepExplore, Why: "w", DoneWhen: "d"}

		_, _, err := m.PatchPlan(2, []PlanPatchOp{{Op: PlanPatchInsertStep, Step: step}}, false)
		require.ErrorContains(t, err, "before or after anchor is required when the plan has steps")

		bi := *step
		_, _, err = m.PatchPlan(
			2,
			[]PlanPatchOp{{Op: PlanPatchInsertStep, Before: "write-schema", After: "decode-legacy", Step: &bi}},
			false,
		)
		require.ErrorContains(t, err, "takes one anchor: before or after, not both")

		af := *step
		_, _, err = m.PatchPlan(2, []PlanPatchOp{{Op: PlanPatchInsertStep, After: "nope", Step: &af}}, false)
		require.ErrorContains(t, err, `patch op 1 (insert_step): step "nope" not found`)

		before := *step
		patched, _, err := m.PatchPlan(
			2,
			[]PlanPatchOp{{Op: PlanPatchInsertStep, Before: "decode-legacy", Step: &before}},
			false,
		)
		require.NoError(t, err)
		assert.Equal(t, "fresh", patched.Items[1].ID, "before puts the step ahead of its anchor")
	})

	t.Run("insert enforces the v2 step contract", func(t *testing.T) {
		m := patchedFixture(t)
		dup := &PlanItem{ID: "write-schema", Content: "c", Type: StepExplore, Why: "w", DoneWhen: "d"}
		_, _, err := m.PatchPlan(2, []PlanPatchOp{{Op: PlanPatchInsertStep, After: "write-schema", Step: dup}}, false)
		require.ErrorContains(t, err, `patch op 1 (insert_step)`)
		require.ErrorContains(t, err, `duplicates id "write-schema"`)

		bare := &PlanItem{ID: "bare", Content: "c", Type: StepExplore}
		_, _, err = m.PatchPlan(2, []PlanPatchOp{{Op: PlanPatchInsertStep, After: "write-schema", Step: bare}}, false)
		require.ErrorContains(t, err, `patch op 1 (insert_step)`)
		require.ErrorContains(t, err, "why is required")
	})

	t.Run("reorder is a full permutation by id", func(t *testing.T) {
		m := patchedFixture(t)

		_, _, err := m.PatchPlan(2, []PlanPatchOp{{Op: PlanPatchReorderSteps, IDs: []string{"decode-legacy"}}}, false)
		require.ErrorContains(t, err, "reorder lists 1 ids for 2 steps")

		_, _, err = m.PatchPlan(
			2,
			[]PlanPatchOp{{Op: PlanPatchReorderSteps, IDs: []string{"decode-legacy", "nope"}}},
			false,
		)
		require.ErrorContains(t, err, `step "nope" not found`)

		_, _, err = m.PatchPlan(
			2,
			[]PlanPatchOp{{Op: PlanPatchReorderSteps, IDs: []string{"decode-legacy", "decode-legacy"}}},
			false,
		)
		require.ErrorContains(t, err, `id "decode-legacy" listed twice`)

		_, summary, err := m.PatchPlan(2, []PlanPatchOp{
			{Op: PlanPatchReorderSteps, IDs: []string{"decode-legacy", "write-schema"}},
		}, false)
		require.NoError(t, err)
		assert.True(t, summary.StepsReordered)
		assert.Equal(t, []string{"decode-legacy", "write-schema"}, stepIDs(m.Plan().Items))

		patched, summary, err := m.PatchPlan(3, []PlanPatchOp{
			{Op: PlanPatchReorderSteps, IDs: []string{"decode-legacy", "write-schema"}},
		}, false)
		require.NoError(t, err)
		assert.False(t, summary.StepsReordered, "a no-op reorder reports no change")
		assert.Equal(t, uint64(4), patched.Revision)
	})
}

// emptyPlanFixture returns a manager holding a v2 plan with no steps at
// revision 1 — the state a session is in before the first step is authored.
func emptyPlanFixture(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	empty := v2Fixture()
	empty.Items = nil
	_, _, err = m.ReplacePlanV2(empty, false)
	require.NoError(t, err)
	require.Empty(t, m.Plan().Items)
	require.Equal(t, uint64(1), m.Plan().Revision)
	return m
}

func TestPatchPlanInsertsTheFirstStepOfAnEmptyPlan(t *testing.T) {
	m := emptyPlanFixture(t)
	first := &PlanItem{ID: "first", Content: "sketch the shape", Type: StepExplore, Why: "w", DoneWhen: "d"}

	patched, summary, err := m.PatchPlan(1, []PlanPatchOp{{Op: PlanPatchInsertStep, Step: first}}, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"first"}, stepIDs(patched.Items))
	assert.Equal(t, PlanPending, patched.Items[0].Status, "an inserted step starts pending")
	assert.Equal(t, []string{"first"}, summary.StepsInserted)

	second := &PlanItem{ID: "second", Content: "then edit", Type: StepEdit, Why: "w", DoneWhen: "d"}
	_, _, err = m.PatchPlan(2, []PlanPatchOp{{Op: PlanPatchInsertStep, Step: second}}, false)
	require.ErrorContains(t, err, "before or after anchor is required when the plan has steps")
	assert.Equal(t, []string{"first"}, stepIDs(m.Plan().Items), "the refused insert leaves the plan alone")

	// The rule reads the candidate plan, not the durable one: inside one batch
	// the second anchorless insert already has a step to be ambiguous about.
	batched := emptyPlanFixture(t)
	_, _, err = batched.PatchPlan(1, []PlanPatchOp{
		{Op: PlanPatchInsertStep, Step: first},
		{Op: PlanPatchInsertStep, Step: second},
	}, false)
	require.ErrorContains(t, err, "patch op 2 (insert_step)")
	require.ErrorContains(t, err, "before or after anchor is required when the plan has steps")
	assert.Empty(t, batched.Plan().Items, "the rolled-back batch leaves the plan empty")
}

func TestPatchPlanDirectiveOps(t *testing.T) {
	m := patchedFixture(t)

	_, _, err := m.PatchPlan(
		2,
		[]PlanPatchOp{{Op: PlanPatchAddConstraint, Value: " no tool API switch in the expand phase "}},
		false,
	)
	require.ErrorContains(t, err, `constraint "no tool API switch in the expand phase" already exists`)

	_, _, err = m.PatchPlan(2, []PlanPatchOp{{Op: PlanPatchRemoveConstraint, Value: "missing"}}, false)
	require.ErrorContains(t, err, `constraint "missing" not found`)

	_, _, err = m.PatchPlan(2, []PlanPatchOp{{Op: PlanPatchUpdateConstraint, From: "missing", To: "x"}}, false)
	require.ErrorContains(t, err, `constraint "missing" not found`)

	_, _, err = m.PatchPlan(2, []PlanPatchOp{
		{Op: PlanPatchUpdateCriterion, From: "legacy files still load", To: "round-trip keeps every field"},
	}, false)
	require.ErrorContains(
		t,
		err,
		`patch op 1 (update_criterion): success criterion "round-trip keeps every field" already exists`,
	)

	patched, summary, err := m.PatchPlan(2, []PlanPatchOp{
		{Op: PlanPatchAddConstraint, Value: "patch is all-or-none"},
		{Op: PlanPatchUpdateConstraint, From: "patch is all-or-none", To: "patch batches are atomic"},
		{Op: PlanPatchRemoveConstraint, Value: "no tool API switch in the expand phase"},
		{Op: PlanPatchAddCriterion, Value: "patch round-trips"},
	}, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"patch batches are atomic"}, patched.Constraints)
	assert.Len(t, patched.SuccessCriteria, 3)
	assert.Equal(t, []string{"constraints", "successCriteria"}, summary.PlanFields)
}

func TestPatchPlanRejectsForeignFieldsAndUnknownOps(t *testing.T) {
	m := patchedFixture(t)

	cases := []struct {
		name string
		ops  []PlanPatchOp
		want string
	}{
		{"unknown op", []PlanPatchOp{{Op: "frobnicate"}}, `unknown op "frobnicate"`},
		{"empty batch", nil, "session: plan patch has no operations"},
		{
			"update_step takes no goal",
			[]PlanPatchOp{{Op: PlanPatchUpdateStep, ID: "decode-legacy", Goal: pv("nope")}},
			"update_step takes no goal",
		},
		{
			"add_constraint takes no ids",
			[]PlanPatchOp{{Op: PlanPatchAddConstraint, Value: "v", IDs: []string{"write-schema"}}},
			"add_constraint takes no ids",
		},
		{
			"op sets nothing",
			[]PlanPatchOp{{Op: PlanPatchSetPlanFields}},
			"sets no fields",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := m.PatchPlan(2, tc.ops, false)
			require.ErrorContains(t, err, tc.want)
		})
	}

	tooMany := make([]PlanPatchOp, maxPlanPatchOps+1)
	for i := range tooMany {
		tooMany[i] = PlanPatchOp{Op: PlanPatchUpdateStep, ID: "decode-legacy", Note: pv("n")}
	}
	_, _, err := m.PatchPlan(2, tooMany, false)
	require.ErrorContains(t, err, "session: plan patch has 33 operations; maximum is 32")
}

func TestPatchPlanKeepsApprovalOnOperationalOnlyChange(t *testing.T) {
	m := patchedFixture(t)

	patched, _, err := m.PatchPlan(2, []PlanPatchOp{
		{Op: PlanPatchUpdateStep, ID: "decode-legacy", Note: pv("note is operational")},
	}, false)
	require.NoError(t, err)
	assert.True(t, patched.Approved, "a note-only patch keeps the user's approval")
	assert.Equal(t, uint64(3), patched.Revision)
}

func stepIDs(items []PlanItem) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return ids
}

// populatedValue builds a non-zero value for one PlanPatchOp field so the
// foreign-field checker can be exercised generically.
func populatedValue(t *testing.T, fieldType reflect.Type) reflect.Value {
	t.Helper()
	switch fieldType.Kind() {
	case reflect.String:
		// Convert so named string types (e.g. PlanResult) also populate.
		return reflect.ValueOf("populated").Convert(fieldType)
	case reflect.Bool:
		return reflect.ValueOf(true)
	case reflect.Slice:
		return reflect.MakeSlice(fieldType, 1, 1)
	case reflect.Pointer:
		return reflect.New(fieldType.Elem())
	case reflect.Struct:
		slot := reflect.New(fieldType).Elem()
		if set := slot.FieldByName("Set"); set.IsValid() && set.Kind() == reflect.Bool {
			set.SetBool(true)
			return slot
		}
	}
	t.Fatalf("unsupported field kind %s for a populated value", fieldType.Kind())
	return reflect.Value{}
}

// TestPatchForeignTableCoversEveryField pins opCheckForeign to the struct: a
// field added to PlanPatchOp but forgotten in the table would be silently
// accepted by every op, so every field the op does not take must be rejected
// and every field it takes must pass.
func TestPatchForeignTableCoversEveryField(t *testing.T) {
	typ := reflect.TypeFor[PlanPatchOp]()
	for i := range typ.NumField() {
		field := typ.Field(i)
		if field.Name == "Op" {
			continue
		}
		op := reflect.ValueOf(&PlanPatchOp{Op: PlanPatchRemoveStep}).Elem()
		op.Field(i).Set(populatedValue(t, field.Type))
		err := op.Interface().(PlanPatchOp).rejectForeignFields("id")
		if field.Name == "ID" {
			require.NoErrorf(t, err, "the op's own field %s must be accepted", field.Name)
			continue
		}
		require.ErrorContainsf(t, err, "takes no", "field %s must be visible to the foreign-field table", field.Name)
	}
}

// TestPatchPlanBatchSeesIntermediateStates: ops apply sequentially to one
// candidate, so a batch that replaces the last success criterion must not be
// validated against the mid-batch state where the plan briefly has none.
func TestPatchPlanBatchSeesIntermediateStates(t *testing.T) {
	m := patchedFixture(t)
	// Drop to exactly one criterion first, so the batch under test crosses
	// the required-at-least-one floor mid-way.
	mustPatch(t, m, []PlanPatchOp{{Op: PlanPatchRemoveCriterion, Value: "legacy files still load"}})

	plan, _, err := m.PatchPlan(m.Plan().Revision, []PlanPatchOp{
		{Op: PlanPatchRemoveCriterion, Value: "round-trip keeps every field"},
		{Op: PlanPatchAddCriterion, Value: "batches apply as one sequential rewrite"},
	}, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"batches apply as one sequential rewrite"}, plan.SuccessCriteria)
}

// mustPatch applies a patch that must succeed.
func mustPatch(t *testing.T, m *Manager, ops []PlanPatchOp) {
	t.Helper()
	_, _, err := m.PatchPlan(m.Plan().Revision, ops, true)
	require.NoError(t, err)
}
