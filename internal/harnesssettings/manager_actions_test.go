package harnesssettings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
)

// Default actions persist in plan.defaults and survive the manager roundtrip:
// what the policy compiled is what lands on disk and what a reopen reads back.
func TestManagerRoundTripsDefaultActions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`plan:
  defaults:
    actions:
      - event: plan_end
        type: compact
    types:
      - name: run
        tools: [bash]
        actions:
          - event: step_start
            type: inject_skill
            skills: [tdd]
`), 0o600))

	runtime, err := plangate.NewRuntime(plangate.DefaultDefaults())
	require.NoError(t, err)
	manager, err := Open(path, runtime, nil)
	require.NoError(t, err)

	planActions := []session.PlanAction{{Event: session.PlanActionOnPlanEnd, Type: session.PlanActionCompact}}
	snapshot := manager.Snapshot()
	assert.Equal(t, planActions, snapshot.Plan.Actions)
	assert.Equal(t, planActions, runtime.Current().PlanActions())
	require.Len(t, snapshot.Plan.Types, 1)
	assert.Equal(t, session.StepRun, snapshot.Plan.Types[0].Name)
	assert.Equal(t, []session.PlanAction{{
		Event:  session.PlanActionOnStepStart,
		Type:   session.PlanActionInjectSkill,
		Skills: []string{"tdd"},
	}}, snapshot.Plan.Types[0].Actions)

	// Apply rewrites the section from the draft; existing actions survive the
	// encode/decode roundtrip and a new one joins them.
	draft := snapshot.Draft()
	draft.Plan.Actions = append(draft.Plan.Actions,
		session.PlanAction{Event: session.PlanActionOnPlanStart, Type: session.PlanActionCompact})
	applied, err := manager.Apply(t.Context(), draft)
	require.NoError(t, err)
	require.Len(t, applied.Plan.Actions, 2)

	reopened, err := Open(path, runtime, nil)
	require.NoError(t, err)
	assert.Equal(t, applied.Plan, reopened.Snapshot().Plan)

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(written), "inject_skill")
	assert.Contains(t, string(written), "step_start")
}

// A draft carrying invalid actions fails Apply without touching the file: the
// error names the offending event so the settings pane can surface it.
func TestManagerApplyRejectsInvalidDefaultActions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(
		t,
		os.WriteFile(path, []byte("plan:\n  defaults:\n    types:\n      - name: run\n        tools: [bash]\n"), 0o600),
	)
	runtime, err := plangate.NewRuntime(plangate.DefaultDefaults())
	require.NoError(t, err)
	manager, err := Open(path, runtime, nil)
	require.NoError(t, err)

	draft := manager.Snapshot().Draft()
	draft.Plan.Actions = []session.PlanAction{{
		Event: session.PlanActionOnStepStart, Type: session.PlanActionCompact,
	}}
	_, err = manager.Apply(t.Context(), draft)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "step_start")
}
