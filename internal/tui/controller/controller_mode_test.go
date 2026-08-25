package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/agent"
	"github.com/pulseaiclub/phi/internal/permission"
)

func gateDecision(t *testing.T, c *Controller, req permission.Request) permission.Decision {
	t.Helper()
	dec, _ := c.gate.Check(t.Context(), req)
	return dec
}

// TestSetModePlanOverlaysReadonlyPolicy pins the plan posture at the gate:
// write/edit and non-allowlisted bash are denied outright, while reads and
// allowlisted checks (git status, go test) keep running.
func TestSetModePlanOverlaysReadonlyPolicy(t *testing.T) {
	policy := permission.DefaultPolicy()
	policy.WorkspaceOnlyWrites = false // path-independent assertions below

	c := &Controller{basePolicy: policy}
	c.initGate(policy)

	writeReq := permission.Request{Action: permission.ActionWrite, Tool: "write", Paths: []string{"/tmp/w/x.txt"}}
	bashReq := permission.Request{Action: permission.ActionBash, Tool: "bash", Command: "pip install numpy"}
	readReq := permission.Request{Action: permission.ActionRead, Tool: "read", Paths: []string{"/tmp/w/x.txt"}}

	assert.Equal(t, permission.Allow, gateDecision(t, c, writeReq), "build mode allows in-workspace writes")
	assert.Equal(t, permission.Ask, gateDecision(t, c, bashReq), "build mode asks for non-allowlisted bash")

	c.SetMode(agent.ModePlan)
	assert.Equal(t, agent.ModePlan, c.Mode())
	assert.Equal(t, permission.Deny, gateDecision(t, c, writeReq), "plan mode denies writes")
	assert.Equal(t, permission.Deny, gateDecision(t, c, bashReq), "plan mode denies non-allowlisted bash")
	assert.Equal(t, permission.Allow, gateDecision(t, c, readReq), "plan mode keeps reads")
	assert.Equal(t, permission.Allow, gateDecision(
		t, c,
		permission.Request{Action: permission.ActionBash, Tool: "bash", Command: "git status"},
	), "plan mode keeps allowlisted checks")

	// Back to build: the configured base policy is restored.
	c.SetMode(agent.ModeBuild)
	assert.Equal(t, permission.Ask, gateDecision(t, c, bashReq))
	assert.Equal(t, permission.Allow, gateDecision(t, c, writeReq))
}

func TestToggleModeCyclesThreeStates(t *testing.T) {
	c := &Controller{}
	require.Equal(t, agent.ModeBuild, c.ToggleMode(), "useplan (default) → build")
	require.Equal(t, agent.ModePlan, c.ToggleMode(), "build → plan")
	require.Equal(t, agent.ModeUsePlan, c.ToggleMode(), "plan → useplan")
}

func TestControllerZeroValueModeIsUsePlan(t *testing.T) {
	var c Controller
	require.Equal(t, agent.ModeUsePlan, c.Mode())
}

func TestSetModeUnknownFallsBackToUsePlan(t *testing.T) {
	c := &Controller{}
	c.SetMode("bogus")
	require.Equal(t, agent.ModeUsePlan, c.Mode())

	c.SetMode(agent.ModeBuild)
	require.Equal(t, agent.ModeBuild, c.Mode(), "build is an explicit mode, not an unknown fallback")
}

func TestSetModeUsePlanKeepsBuildPermissions(t *testing.T) {
	policy := permission.DefaultPolicy()
	policy.WorkspaceOnlyWrites = false
	c := &Controller{basePolicy: policy}
	c.initGate(policy)

	c.SetMode(agent.ModeUsePlan)
	require.Equal(t, agent.ModeUsePlan, c.Mode())

	writeReq := permission.Request{Action: permission.ActionWrite, Tool: "write", Paths: []string{"/tmp/w/x.txt"}}
	assert.Equal(t, permission.Allow, gateDecision(t, c, writeReq), "useplan must not overlay readonly")
}

// TestSetModeSurvivesGateReinit: SetModel re-initializes the gate from config;
// an active plan overlay must survive that rebuild.
func TestSetModeSurvivesGateReinit(t *testing.T) {
	c := &Controller{}
	c.basePolicy = permission.DefaultPolicy()
	c.initGate(c.basePolicy)

	c.SetMode(agent.ModePlan)
	c.initGate(permission.DefaultPolicy()) // what SetModel does on model change

	assert.Equal(t, permission.Deny, gateDecision(
		t, c,
		permission.Request{Action: permission.ActionEdit, Tool: "edit", Paths: []string{"/tmp/w/x.txt"}},
	))
}

// TestSetModeNilEngineNoPanic: mode switching before the engine exists (or in
// degenerate tests) must not panic.
func TestSetModeNilEngineNoPanic(t *testing.T) {
	c := &Controller{}
	require.NotPanics(t, func() { c.SetMode(agent.ModePlan) })
	assert.Equal(t, agent.ModePlan, c.Mode())
}
