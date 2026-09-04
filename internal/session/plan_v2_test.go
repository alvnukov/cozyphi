package session

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func v2Fixture() PlanV2 {
	return PlanV2{
		Goal:            "ship the plan v2 durable contract",
		Approach:        "extend the durable structs behind one normalize path",
		SuccessCriteria: []string{"round-trip keeps every field", "legacy files still load"},
		Constraints:     []string{"no tool API switch in the expand phase"},
		WorkingContext:  "internal/session/plan.go is the only plan writer",
		Items: []PlanItem{
			{
				ID:           "write-schema",
				Content:      "extend Plan and PlanItem with v2 contract fields",
				Status:       PlanCompleted,
				Type:         StepEdit,
				Note:         "kept operational",
				Evidence:     "focused session tests",
				Why:          "the contract needs somewhere durable to live",
				DoneWhen:     "fields persist and restore",
				Outcome:      "schema landed",
				EvidenceRefs: []string{"internal/session/plan.go", "internal/session/load.go"},
			},
			{
				ID:       "decode-legacy",
				Content:  "decode legacy snapshots into the same canonical shape",
				Status:   PlanInProgress,
				Type:     StepRun,
				Why:      "old sessions must resume without data loss",
				DoneWhen: "legacy entry loads without panic",
				Risk:     "touches the session load path",
				JIT:      true,
			},
		},
	}
}

func TestReplacePlanV2RoundTripsContractFields(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)

	created, _, _, err := m.ReplacePlanV2(v2Fixture(), false)
	require.NoError(t, err)
	assert.Equal(t, PlanSchemaV2, created.Schema)
	assert.Equal(t, uint64(1), created.Revision)
	assert.Empty(t, created.Result)
	assert.Nil(t, created.ClosedAt)

	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	restored := loaded.Plan()
	assert.Equal(t, PlanSchemaV2, restored.Schema)
	assert.Equal(t, created.Revision, restored.Revision)
	assert.Equal(t, created.Goal, restored.Goal)
	assert.Equal(t, created.Approach, restored.Approach)
	assert.Equal(t, created.SuccessCriteria, restored.SuccessCriteria)
	assert.Equal(t, created.Constraints, restored.Constraints)
	assert.Equal(t, created.WorkingContext, restored.WorkingContext)
	assert.Equal(t, created.Items, restored.Items)
	assert.True(t, created.UpdatedAt.Equal(restored.UpdatedAt))

	again, err := OpenSession(m.File())
	require.NoError(t, err)
	assert.Equal(t, restored.Items, again.Plan().Items, "reloading must be stable")
}

func TestReplacePlanV2RecordsResultMetadata(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)

	closed := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	fixture := v2Fixture()
	fixture.Items[0].Status = PlanCompleted
	fixture.Items[1].Status = PlanCancelled
	fixture.Result = PlanResultSuccess
	fixture.ClosedAt = &closed

	created, _, _, err := m.ReplacePlanV2(fixture, false)
	require.NoError(t, err)
	assert.Equal(t, PlanResultSuccess, created.Result)
	require.NotNil(t, created.ClosedAt)
	assert.True(t, closed.Equal(*created.ClosedAt))
	assert.False(t, created.Approved, "a plan with no active work closes approval")

	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	restored := loaded.Plan()
	assert.Equal(t, PlanResultSuccess, restored.Result)
	require.NotNil(t, restored.ClosedAt)
	assert.True(t, closed.Equal(*restored.ClosedAt))
}

func TestReplacePlanV2RequiresContractFields(t *testing.T) {
	m := NewManager(t.TempDir())
	valid, _, _, err := m.ReplacePlanV2(v2Fixture(), false)
	require.NoError(t, err)

	cases := map[string]func(*PlanV2){
		"missing goal":             func(p *PlanV2) { p.Goal = " " },
		"missing approach":         func(p *PlanV2) { p.Approach = "" },
		"missing criteria":         func(p *PlanV2) { p.SuccessCriteria = nil },
		"blank criterion":          func(p *PlanV2) { p.SuccessCriteria[1] = "  " },
		"missing step action":      func(p *PlanV2) { p.Items[1].Content = " " },
		"missing step id":          func(p *PlanV2) { p.Items[1].ID = "" },
		"uppercase step id":        func(p *PlanV2) { p.Items[1].ID = "Decode-Legacy" },
		"too long step id":         func(p *PlanV2) { p.Items[1].ID = strings.Repeat("a", maxPlanStepIDRunes+1) },
		"duplicate step ids":       func(p *PlanV2) { p.Items[1].ID = p.Items[0].ID },
		"missing step why":         func(p *PlanV2) { p.Items[1].Why = "" },
		"missing step done_when":   func(p *PlanV2) { p.Items[1].DoneWhen = " " },
		"unknown result":           func(p *PlanV2) { p.Result = "finished" },
		"result without closed_at": func(p *PlanV2) { p.Result = PlanResultAbandoned },
		"closed_at without result": func(p *PlanV2) { p.ClosedAt = new(time.Now()) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := v2Fixture()
			mutate(&fixture)
			_, _, _, err := m.ReplacePlanV2(fixture, false)
			require.Error(t, err)
			assert.Equal(t, valid, m.Plan(), "a rejected snapshot must not mutate durable state")
		})
	}
}

func repeatEntries(prefix string, n int) []string {
	entries := make([]string, n)
	for i := range entries {
		entries[i] = prefix + "-" + strconv.Itoa(i)
	}
	return entries
}

// Every prose field carries two rungs — a norm and a hard ceiling at five
// times it — so this table refuses only above the hard value; the over-norm
// rung between them is accepted with an advisory and covered by
// TestReplacePlanV2SoftLimitsCoverThreeRanges below.
func TestReplacePlanV2EnforcesFieldBounds(t *testing.T) {
	m := NewManager(t.TempDir())
	cases := map[string]func(*PlanV2){
		"too long goal":     func(p *PlanV2) { p.Goal = strings.Repeat("g", maxPlanGoalHardRunes+1) },
		"too long approach": func(p *PlanV2) { p.Approach = strings.Repeat("a", maxPlanApproachHardRunes+1) },
		"too long criterion": func(p *PlanV2) {
			p.SuccessCriteria = []string{strings.Repeat("c", maxPlanDirectiveHardRunes+1)}
		},
		"too many criteria": func(p *PlanV2) {
			p.SuccessCriteria = repeatEntries("criterion", maxPlanDirectiveEntries+1)
		},
		"too long constraint": func(p *PlanV2) {
			p.Constraints = []string{strings.Repeat("c", maxPlanDirectiveHardRunes+1)}
		},
		"too many constraints": func(p *PlanV2) {
			p.Constraints = repeatEntries("constraint", maxPlanDirectiveEntries+1)
		},
		"too long working context": func(p *PlanV2) {
			p.WorkingContext = strings.Repeat("w", maxPlanWorkingContextHardRunes+1)
		},
		"too long why":       func(p *PlanV2) { p.Items[1].Why = strings.Repeat("y", maxPlanStepWhyHardRunes+1) },
		"too long done_when": func(p *PlanV2) { p.Items[1].DoneWhen = strings.Repeat("d", maxPlanStepDoneWhenHardRunes+1) },
		"too long outcome":   func(p *PlanV2) { p.Items[1].Outcome = strings.Repeat("o", maxPlanStepOutcomeHardRunes+1) },
		"too long risk":      func(p *PlanV2) { p.Items[1].Risk = strings.Repeat("r", maxPlanStepRiskHardRunes+1) },
		"too long evidence ref": func(p *PlanV2) {
			p.Items[1].EvidenceRefs = []string{strings.Repeat("e", maxPlanEvidenceRefHardRunes+1)}
		},
		"too many evidence refs": func(p *PlanV2) {
			p.Items[1].EvidenceRefs = repeatEntries("ref", maxPlanEvidenceRefsPerStep+1)
		},
		"serialized plan over budget": func(p *PlanV2) {
			// Every prose field at its hard cap: per-field bounds accept, but
			// the whole snapshot must still cross the serialized ceiling.
			p.Items = make([]PlanItem, maxPlanItems)
			for i := range p.Items {
				p.Items[i] = PlanItem{
					ID:           "bulk-step-" + strconv.Itoa(i),
					Content:      strings.Repeat("c", maxPlanContentHardRunes),
					Status:       PlanPending,
					Type:         StepEdit,
					Note:         strings.Repeat("n", maxPlanNoteHardRunes),
					Evidence:     strings.Repeat("e", maxPlanEvidenceHardRunes),
					Why:          strings.Repeat("y", maxPlanStepWhyHardRunes),
					DoneWhen:     strings.Repeat("d", maxPlanStepDoneWhenHardRunes),
					Outcome:      strings.Repeat("o", maxPlanStepOutcomeHardRunes),
					Risk:         strings.Repeat("r", maxPlanStepRiskHardRunes),
					EvidenceRefs: make([]string, maxPlanEvidenceRefsPerStep),
				}
				for j := range p.Items[i].EvidenceRefs {
					p.Items[i].EvidenceRefs[j] = strings.Repeat("f", maxPlanEvidenceRefHardRunes)
				}
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := v2Fixture()
			mutate(&fixture)
			_, _, _, err := m.ReplacePlanV2(fixture, false)
			require.Error(t, err)
		})
	}
}

// TestReplacePlanV2SoftLimitsCoverThreeRanges pins the advisory rung on
// representative prose — one plan-level field, one step field, one directive:
// at norm the write is silent, one rune over norm it lands with exactly one
// advisory naming the field and the norm, and one rune over hard it is
// refused naming the hard ceiling.
func TestReplacePlanV2SoftLimitsCoverThreeRanges(t *testing.T) {
	m := NewManager(t.TempDir())
	fields := map[string]struct {
		norm, hard int
		mutate     func(*PlanV2, string)
	}{
		"working context": {
			maxPlanWorkingContextRunes, maxPlanWorkingContextHardRunes,
			func(p *PlanV2, s string) { p.WorkingContext = s },
		},
		"item 2 content": {
			maxPlanContentRunes, maxPlanContentHardRunes,
			func(p *PlanV2, s string) { p.Items[1].Content = s },
		},
		"success criterion 1": {
			maxPlanDirectiveRunes, maxPlanDirectiveHardRunes,
			func(p *PlanV2, s string) { p.SuccessCriteria[0] = s },
		},
	}
	for name, f := range fields {
		t.Run(name, func(t *testing.T) {
			atNorm := v2Fixture()
			f.mutate(&atNorm, strings.Repeat("x", f.norm))
			_, _, advisories, err := m.ReplacePlanV2(atNorm, false)
			require.NoError(t, err)
			assert.Empty(t, advisories, "within norm the write is silent")

			overNorm := v2Fixture()
			f.mutate(&overNorm, strings.Repeat("x", f.norm+1))
			_, _, advisories, err = m.ReplacePlanV2(overNorm, false)
			require.NoError(t, err, "between the rungs the write still lands")
			require.Len(t, advisories, 1)
			assert.Contains(t, advisories[0], name)
			assert.Contains(t, advisories[0], fmt.Sprintf("over the %d norm", f.norm))

			overHard := v2Fixture()
			f.mutate(&overHard, strings.Repeat("x", f.hard+1))
			_, _, _, err = m.ReplacePlanV2(overHard, false)
			require.ErrorContains(t, err, fmt.Sprintf("exceeds %d characters", f.hard))
		})
	}
}

func TestReplacePlanV2DropsApprovalOnlyOnContractChange(t *testing.T) {
	m := NewManager(t.TempDir())
	created, _, _, err := m.ReplacePlanV2(v2Fixture(), true)
	require.NoError(t, err)
	require.True(t, created.Approved)

	operational := v2Fixture()
	operational.Items[1].Outcome = "half done"
	operational.Items[1].Note = "progress note"
	kept, _, _, err := m.ReplacePlanV2(operational, false)
	require.NoError(t, err)
	assert.True(t, kept.Approved, "operational metadata must not reset approval")

	jitFlip := v2Fixture()
	jitFlip.Items[1].JIT = false
	flipped, _, _, err := m.ReplacePlanV2(jitFlip, false)
	require.NoError(t, err)
	assert.False(t, flipped.Approved, "flipping the just-in-time approval posture is a contract change")

	contract := v2Fixture()
	contract.Goal = "a different goal"
	dropped, _, _, err := m.ReplacePlanV2(contract, false)
	require.NoError(t, err)
	assert.False(t, dropped.Approved, "a contract change must reset approval")

	stepContract := v2Fixture()
	stepContract.Items[1].DoneWhen = "a different exit condition"
	droppedAgain, _, _, err := m.ReplacePlanV2(stepContract, false)
	require.NoError(t, err)
	assert.False(t, droppedAgain.Approved)
}

func TestApprovalAndRenamesPreserveV2Contract(t *testing.T) {
	m := NewManager(t.TempDir())
	created, _, _, err := m.ReplacePlanV2(v2Fixture(), false)
	require.NoError(t, err)

	approved, err := m.SetPlanApproved(true)
	require.NoError(t, err)
	assert.Equal(t, created.Goal, approved.Goal)
	assert.Equal(t, created.Approach, approved.Approach)
	assert.Equal(t, created.SuccessCriteria, approved.SuccessCriteria)
	assert.Equal(t, created.Constraints, approved.Constraints)
	assert.Equal(t, created.WorkingContext, approved.WorkingContext)
	assert.Equal(t, created.Items, approved.Items)

	renamed, err := m.RenamePlanStepTypes(map[StepType]StepType{StepEdit: StepRun})
	require.NoError(t, err)
	assert.Equal(t, created.Items[0].ID, renamed.Items[0].ID)
	assert.Equal(t, created.Items[0].Why, renamed.Items[0].Why)
	assert.Equal(t, created.Items[0].DoneWhen, renamed.Items[0].DoneWhen)
	assert.Equal(t, created.Items[0].EvidenceRefs, renamed.Items[0].EvidenceRefs)
	assert.Equal(t, created.Goal, renamed.Goal)

	cleared, err := m.ClearPlan()
	require.NoError(t, err)
	assert.False(t, cleared.Schema.IsV2())
	assert.Empty(t, cleared.Goal)
	assert.Empty(t, cleared.Items)
}

func TestLegacyReplaceStripsModelSuppliedV2StepFields(t *testing.T) {
	m := NewManager(t.TempDir())
	plan, err := m.ReplacePlan([]PlanItem{{
		Content:      "legacy step",
		Status:       PlanPending,
		Type:         StepEdit,
		ID:           "smuggled",
		Why:          "model authored",
		DoneWhen:     "model authored",
		Outcome:      "model authored",
		Risk:         "model authored",
		JIT:          true,
		EvidenceRefs: []string{"model authored"},
	}})
	require.NoError(t, err)
	assert.Equal(t, PlanSchemaLegacy, plan.Schema)
	assert.Equal(t, []PlanItem{{Content: "legacy step", Status: PlanPending, Type: StepEdit}}, plan.Items)
}

func TestOpenSessionLoadsLegacyPlanIntoCanonicalShape(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)

	now := time.Now()
	legacy := Plan{
		Revision:  4,
		UpdatedAt: now,
		Approved:  true,
		Items: []PlanItem{
			{Content: "first", Status: PlanCompleted, Type: StepExplore, Note: "kept", Evidence: "kept"},
			{Content: "second", Status: PlanInProgress, Type: StepEdit},
		},
	}
	entry := PlanEntry{
		SessionBaseEntry: SessionBaseEntry{Type: EntryPlan, ID: m.generateID(), Timestamp: now},
		Plan:             legacy,
	}
	m.entries = append(m.entries, entry)
	m.byIDs[entry.ID] = entry
	require.NoError(t, m.flush(entry))

	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	restored := loaded.Plan()
	assert.Equal(t, uint64(4), restored.Revision)
	assert.True(t, restored.Approved)
	assert.Equal(t, legacy.Items, restored.Items, "legacy items load without loss or invention")
	assert.False(t, restored.Schema.IsV2())
	assert.Empty(t, restored.Goal)
	assert.Empty(t, restored.Result)
	assert.Nil(t, restored.ClosedAt)
}

func TestOpenSessionRejectsUnknownPlanSchema(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)

	now := time.Now()
	entry := PlanEntry{
		SessionBaseEntry: SessionBaseEntry{Type: EntryPlan, ID: m.generateID(), Timestamp: now},
		Plan: Plan{
			Schema:    PlanSchema(7),
			UpdatedAt: now,
			Items:     []PlanItem{{Content: "step", Status: PlanPending}},
		},
	}
	m.entries = append(m.entries, entry)
	m.byIDs[entry.ID] = entry
	require.NoError(t, m.flush(entry))

	_, err = OpenSession(m.File())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema")
}

func TestOpenSessionRejectsOversizedV2Plan(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)

	refs := make([]string, maxPlanEvidenceRefsPerStep)
	for j := range refs {
		refs[j] = strings.Repeat("f", maxPlanEvidenceRefHardRunes)
	}
	items := make([]PlanItem, legacyMaxPlanItems)
	for i := range items {
		items[i] = PlanItem{
			ID:           "oversized-" + strconv.Itoa(i),
			Content:      strings.Repeat("c", maxPlanContentHardRunes),
			Status:       PlanPending,
			Type:         StepEdit,
			Note:         strings.Repeat("n", maxPlanNoteHardRunes),
			Evidence:     strings.Repeat("e", maxPlanEvidenceHardRunes),
			Why:          strings.Repeat("y", maxPlanStepWhyHardRunes),
			DoneWhen:     strings.Repeat("d", maxPlanStepDoneWhenHardRunes),
			Outcome:      strings.Repeat("o", maxPlanStepOutcomeHardRunes),
			Risk:         strings.Repeat("r", maxPlanStepRiskHardRunes),
			EvidenceRefs: refs,
		}
	}
	now := time.Now()
	entry := PlanEntry{
		SessionBaseEntry: SessionBaseEntry{Type: EntryPlan, ID: m.generateID(), Timestamp: now},
		Plan:             Plan{Schema: PlanSchemaV2, UpdatedAt: now, Items: items},
	}
	m.entries = append(m.entries, entry)
	m.byIDs[entry.ID] = entry
	require.NoError(t, m.flush(entry))

	_, err = OpenSession(m.File())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bytes")
}

// TestReplacePlanV2RoundTripsNearSerializedBudget proves the raised write
// budget is also the load budget: a snapshot sized between the old 96K
// ceiling and the new one persists through the real write door and reopens
// without a complaint, byte-identical in shape.
func TestReplacePlanV2RoundTripsNearSerializedBudget(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)

	// Sixteen steps with every prose field at its hard cap land the snapshot
	// around 370K: far past the old 96K budget, safely inside the new one.
	refs := make([]string, maxPlanEvidenceRefsPerStep)
	for j := range refs {
		refs[j] = strings.Repeat("f", maxPlanEvidenceRefHardRunes)
	}
	fixture := v2Fixture()
	for len(fixture.Items) < 16 {
		fixture.Items = append(fixture.Items, PlanItem{
			ID:           "budget-step-" + strconv.Itoa(len(fixture.Items)),
			Content:      strings.Repeat("c", maxPlanContentHardRunes),
			Status:       PlanPending,
			Type:         StepEdit,
			Note:         strings.Repeat("n", maxPlanNoteHardRunes),
			Evidence:     strings.Repeat("e", maxPlanEvidenceHardRunes),
			Why:          strings.Repeat("y", maxPlanStepWhyHardRunes),
			DoneWhen:     strings.Repeat("d", maxPlanStepDoneWhenHardRunes),
			Outcome:      strings.Repeat("o", maxPlanStepOutcomeHardRunes),
			Risk:         strings.Repeat("r", maxPlanStepRiskHardRunes),
			EvidenceRefs: refs,
		})
	}

	created, _, _, err := m.ReplacePlanV2(fixture, false)
	require.NoError(t, err, "a plan within the write budget must land")
	encoded, err := json.Marshal(created)
	require.NoError(t, err)
	assert.Greater(t, len(encoded), 96*1024, "the snapshot must sit past the old budget")
	assert.LessOrEqual(t, len(encoded), maxPlanV2SerializedBytes)

	reopened, err := OpenSession(m.File())
	require.NoError(t, err, "load accepts everything this harness writes")
	restored := reopened.Plan()
	assert.Equal(t, created.Items, restored.Items, "reloading must be stable")
	assert.Equal(t, created.WorkingContext, restored.WorkingContext)
}
