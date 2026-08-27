package harnesssettings_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
)

type fakePlanMigrator struct {
	plan session.Plan
	err  error
}

func (m *fakePlanMigrator) Plan() session.Plan { return m.plan.Clone() }

func (m *fakePlanMigrator) RenamePlanStepTypes(
	_ context.Context,
	renames map[session.StepType]session.StepType,
) (session.Plan, error) {
	if m.err != nil {
		return session.Plan{}, m.err
	}
	for i := range m.plan.Items {
		if renamed, ok := renames[m.plan.Items[i].Type]; ok {
			m.plan.Items[i].Type = renamed
		}
	}
	m.plan.Revision++
	return m.plan.Clone(), nil
}

func TestManagerRenamesTypesInCurrentPlanBeforePublishingPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(
		t,
		os.WriteFile(
			path,
			[]byte("plan:\n  defaults:\n    types:\n      - name: inspect\n        tools: [read]\n"),
			0o600,
		),
	)
	runtime, err := plangate.NewRuntime(plangate.DefaultDefaults())
	require.NoError(t, err)
	plans := &fakePlanMigrator{plan: session.Plan{Approved: true, Items: []session.PlanItem{{
		Content: "inspect", Status: session.PlanInProgress, Type: "inspect",
	}}}}
	manager, err := harnesssettings.Open(path, runtime, plans)
	require.NoError(t, err)

	draft := manager.Snapshot().Draft()
	draft.Plan.Types[0].Name = "review"
	draft.TypeRenames = map[session.StepType]session.StepType{"inspect": "review"}
	_, err = manager.Apply(t.Context(), draft)
	require.NoError(t, err)
	assert.Equal(t, session.StepType("review"), plans.Plan().Items[0].Type)
	assert.Equal(t, []string{"review"}, runtime.Current().StepTypes())
	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(written), "name: review")
}

func TestManagerBlocksDeletingTypeUsedByCurrentPlan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := []byte("plan:\n  defaults:\n    types:\n      - name: inspect\n        tools: [read]\n")
	require.NoError(t, os.WriteFile(path, original, 0o600))
	runtime, err := plangate.NewRuntime(plangate.DefaultDefaults())
	require.NoError(t, err)
	plans := &fakePlanMigrator{
		plan: session.Plan{Items: []session.PlanItem{{Type: "inspect", Status: session.PlanPending}}},
	}
	manager, err := harnesssettings.Open(path, runtime, plans)
	require.NoError(t, err)
	draft := manager.Snapshot().Draft()
	draft.Plan.Types = nil

	_, err = manager.Apply(t.Context(), draft)
	require.ErrorIs(t, err, harnesssettings.ErrTypeInUse)
	written, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, original, written)
	assert.Equal(t, []string{"inspect"}, runtime.Current().StepTypes())
}

func TestManagerLeavesConfigAndRuntimeUntouchedWhenPlanMigrationFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := []byte("plan:\n  defaults:\n    types:\n      - name: inspect\n        tools: [read]\n")
	require.NoError(t, os.WriteFile(path, original, 0o600))
	runtime, err := plangate.NewRuntime(plangate.DefaultDefaults())
	require.NoError(t, err)
	plans := &fakePlanMigrator{
		plan: session.Plan{Items: []session.PlanItem{{Type: "inspect", Status: session.PlanPending}}},
		err:  errors.New("cannot persist current plan"),
	}
	manager, err := harnesssettings.Open(path, runtime, plans)
	require.NoError(t, err)
	draft := manager.Snapshot().Draft()
	draft.Plan.Types[0].Name = "review"
	draft.TypeRenames = map[session.StepType]session.StepType{"inspect": "review"}

	_, err = manager.Apply(t.Context(), draft)
	require.ErrorContains(t, err, "cannot persist current plan")
	written, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, original, written)
	assert.Equal(t, []string{"inspect"}, runtime.Current().StepTypes())
}

func TestManagerAppliesPlanDefaultsWithoutLosingUnrelatedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`# keep this comment
models:
  - name: model-a
    api_key: secret
permissions:
  mode: interactive
plan:
  defaults:
    types:
      - name: inspect
        tools: [read]
`), 0o600))
	runtime, err := plangate.NewRuntime(plangate.DefaultDefaults())
	require.NoError(t, err)
	manager, err := harnesssettings.Open(path, runtime, nil)
	require.NoError(t, err)

	snapshot := manager.Snapshot()
	require.Len(t, snapshot.Plan.Types, 1)
	assert.Equal(t, session.StepType("inspect"), snapshot.Plan.Types[0].Name)

	// An unrelated external edit is merged because only the plan.defaults
	// section participates in optimistic conflict detection.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, []byte("agents:\n  enabled: false\n")...), 0o600))

	draft := snapshot.Draft()
	draft.Plan.Types = append(draft.Plan.Types, plangate.TypeDefaults{Name: "change", Tools: []string{"edit"}})
	applied, err := manager.Apply(t.Context(), draft)
	require.NoError(t, err)
	assert.NotEqual(t, snapshot.Token, applied.Token)

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(written)
	assert.Contains(t, text, "# keep this comment")
	assert.Contains(t, text, "api_key: secret")
	assert.Contains(t, text, "enabled: false")
	assert.Contains(t, text, "name: change")

	plan := session.Plan{Approved: true, Items: []session.PlanItem{{
		Content: "change", Status: session.PlanInProgress, Type: "change",
	}}}
	checker := plangate.Checker{Phase: plangate.PhaseDeny, Runtime: runtime}
	assert.False(t, checker.Check(plan, plangate.ToolCall{Name: "edit", PlanStep: 1}).Deny,
		"durable apply publishes the policy for the next check")
}

func TestManagerRejectsConcurrentPlanDefaultsEdit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("plan:\n  defaults:\n    types: []\n"), 0o600))
	runtime, err := plangate.NewRuntime(plangate.DefaultDefaults())
	require.NoError(t, err)
	manager, err := harnesssettings.Open(path, runtime, nil)
	require.NoError(t, err)
	before := runtime.Current().Defaults()
	draft := manager.Snapshot().Draft()
	draft.Plan.Types = []plangate.TypeDefaults{{Name: "local", Tools: []string{"read"}}}

	require.NoError(
		t,
		os.WriteFile(
			path,
			[]byte("plan:\n  defaults:\n    types:\n      - name: external\n        tools: [grep]\n"),
			0o600,
		),
	)
	_, err = manager.Apply(t.Context(), draft)
	require.Error(t, err)
	assert.ErrorIs(t, err, harnesssettings.ErrConflict)
	assert.Equal(t, before, runtime.Current().Defaults(),
		"a rejected draft must not change live policy")
}
