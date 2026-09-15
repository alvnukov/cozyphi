package plangate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

// The pre-plan exemption set is configuration, not law. Everything except
// plan — the one exemption without which the model could neither read nor
// repair a plan — ships as a default the user may drop. Watch ships dropped:
// its start action executes shell, and on 2026-09-14 it ran a command in a
// session with no plan at all (incident watch-command-start-bypasses-plan-gate).
func TestDefaultExemptionsAreConfigurableWithPlanFloor(t *testing.T) {
	policy, err := Compile(DefaultDefaults())
	require.NoError(t, err)

	exempt := map[string]bool{}
	for _, name := range policy.ExemptTools() {
		exempt[name] = true
	}
	for _, name := range []string{"plan", "context", "harness", "memory", "question", "session", "shell_task", "task"} {
		assert.True(t, exempt[name], "%s ships in the default exemption set", name)
	}
	assert.False(t, exempt["watch"], "watch no longer ships exempt: its start executes shell")

	// An explicit list is the whole editable set: an absent name loses its
	// exemption, and plan stays no matter what the list says.
	lean, err := Compile(Defaults{Exemptions: []string{"task"}})
	require.NoError(t, err)
	leanSet := map[string]bool{}
	for _, name := range lean.ExemptTools() {
		leanSet[name] = true
	}
	assert.True(t, leanSet["plan"], "plan is the non-disableable floor")
	assert.True(t, leanSet["task"])
	assert.False(t, leanSet["memory"], "an absent name loses its exemption")

	// The gate agrees with the set: unapproved plan, deny phase.
	noPlan := session.Plan{}
	assert.False(t, lean.Check(PhaseDeny, noPlan, ToolCall{Name: "task"}).Deny,
		"task stays exempt")
	gated := lean.Check(PhaseDeny, noPlan, ToolCall{Name: "memory"})
	assert.True(t, gated.Miss, "memory is gated once the config drops it")
	assert.True(t, gated.Deny)

	// A user may re-add watch; it then passes the tool gate again — its
	// mutating actions stay pre-plan read-only, covered separately.
	readded, err := Compile(Defaults{Exemptions: []string{"watch"}})
	require.NoError(t, err)
	assert.False(t, readded.Check(PhaseDeny, noPlan, ToolCall{Name: "watch"}).Deny,
		"a re-added exemption passes the tool gate")
}

// The floor holds at compile time: plan never comes from the editable list,
// and only real tools do.
func TestCompileValidatesExemptionList(t *testing.T) {
	_, err := Compile(Defaults{Exemptions: []string{"plan", "memory"}})
	require.ErrorContains(t, err, "mandatory exemption")

	_, err = Compile(Defaults{Exemptions: []string{"memory", "memory"}})
	require.ErrorContains(t, err, "duplicate")

	_, err = Compile(Defaults{Exemptions: []string{"curl"}})
	require.ErrorContains(t, err, `unknown tool "curl"`)

	_, err = Compile(Defaults{
		Types:      []TypeDefaults{{Name: "explore", Tools: []string{"read"}}},
		Exemptions: []string{"read"},
	})
	require.ErrorContains(t, err, "must not also be assigned")
}

// Before the plan is approved, exempt tools run read actions and the
// bookkeeping the work needs to reach a plan — task create/start/note,
// context compact, session set_title — while their mutating actions wait
// for approval. This is the second half of the 2026-09-14 fix: even with
// watch re-exempted in config, its start (a shell command) must not run
// in a session with no approved plan.
func TestPreApprovalExemptToolsRunReadActionsOnly(t *testing.T) {
	readPasses := func(t *testing.T, p *Policy, tool, action string) {
		t.Helper()
		v := p.Check(PhaseDeny, session.Plan{}, ToolCall{Name: tool, Action: action})
		assert.False(t, v.Miss, "%s %s should pass before approval", tool, action)
		assert.False(t, v.Deny, "%s %s should pass before approval", tool, action)
	}
	mutatingDenied := func(t *testing.T, p *Policy, tool, action string) {
		t.Helper()
		v := p.Check(PhaseDeny, session.Plan{}, ToolCall{Name: tool, Action: action})
		assert.True(t, v.Miss, "%s %s must wait for approval", tool, action)
		assert.True(t, v.Deny, "%s %s must wait for approval", tool, action)
		assert.Equal(t, ReasonPlanNotApproved, v.Reason,
			"the controller keys its resume signal on this exact reason")
	}

	defaultSet := DefaultPolicy()
	withWatch, err := Compile(Defaults{Exemptions: []string{"watch"}})
	require.NoError(t, err)

	// Reads and the confirmed exceptions pass before approval.
	readPasses(t, defaultSet, "memory", "list")
	readPasses(t, defaultSet, "task", "current")
	readPasses(t, defaultSet, "shell_task", "list")
	readPasses(t, defaultSet, "task", "create")
	readPasses(t, defaultSet, "task", "start")
	readPasses(t, defaultSet, "task", "note")
	readPasses(t, defaultSet, "context", "compact")
	readPasses(t, defaultSet, "session", "set_title")
	readPasses(t, defaultSet, "question", "ask")
	readPasses(t, defaultSet, "plan", "create")
	readPasses(t, withWatch, "watch", "list")

	// Mutating actions wait for approval — even on a re-exempted watch.
	mutatingDenied(t, defaultSet, "memory", "forget")
	mutatingDenied(t, defaultSet, "shell_task", "stop")
	mutatingDenied(t, defaultSet, "task", "done")
	mutatingDenied(t, defaultSet, "task", "block")
	mutatingDenied(t, withWatch, "watch", "start")
	mutatingDenied(t, withWatch, "watch", "stop")

	// Approval restores the full set.
	approved := session.Plan{Approved: true, Items: []session.PlanItem{
		{ID: "s", Content: "work", Status: session.PlanInProgress, Type: session.StepExplore},
	}}
	cases := []struct {
		p    *Policy
		call ToolCall
	}{
		{defaultSet, ToolCall{Name: "memory", Action: "forget"}},
		{defaultSet, ToolCall{Name: "shell_task", Action: "stop"}},
		{defaultSet, ToolCall{Name: "task", Action: "done"}},
		{withWatch, ToolCall{Name: "watch", Action: "start"}},
	}
	for _, tc := range cases {
		v := tc.p.Check(PhaseDeny, approved, tc.call)
		assert.False(t, v.Miss, "%s %s runs again once the plan is approved", tc.call.Name, tc.call.Action)
		assert.False(t, v.Deny)
	}
}
