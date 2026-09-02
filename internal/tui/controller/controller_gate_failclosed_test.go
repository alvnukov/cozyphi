package controller

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/permission"
)

// The reachable assembly failure: the process working directory disappears, so
// the workspace root resolves to nothing and both the configured policy and
// the built-in default fail to compile. The session must deny, not run
// unguarded.
func TestInitGateDeniesWhenAssemblyFails(t *testing.T) {
	// A root that cannot be resolved is what a lost working directory leaves
	// behind, so both the configured policy and the built-in default fail to
	// compile.
	c := &Controller{workspaceRootFn: func() string { return "workspace/is/gone" }}

	c.initGate(permission.DefaultPolicy())

	require.NotEmpty(t, c.GateFailure(), "the failure must be reported to the UI")
	dec, reason := c.currentGate().Check(t.Context(), permission.Request{
		Action:  permission.ActionBash,
		Command: "ls",
	})
	assert.Equal(t, permission.Deny, dec, "a session without rules must deny every call")
	assert.Contains(t, reason, "permission gate failed to assemble")
}

// The documented exception survives: an explicit dangerously_allow_all still
// bypasses, even when the policy behind it could not be compiled. Nothing else
// may return Allow.
func TestInitGateFailureStillHonorsExplicitBypass(t *testing.T) {
	policy := permission.DefaultPolicy()
	policy.DangerouslyAllowAll = true
	c := &Controller{workspaceRootFn: func() string { return "workspace/is/gone" }}

	c.initGate(policy)

	dec, _ := c.currentGate().Check(t.Context(), permission.Request{
		Action:  permission.ActionBash,
		Command: "ls",
	})
	assert.Equal(t, permission.Allow, dec)
}

// A successful assembly clears a previous failure: the UI must not keep
// warning about a boundary that has since been rebuilt.
func TestInitGateClearsTheFailureOnceTheGateIsReal(t *testing.T) {
	c := &Controller{gateFailure: "stale failure"}

	c.initGate(permission.DefaultPolicy())

	assert.Empty(t, c.GateFailure())
}

// SetModel and SetMode re-run initGate while a run may be judging calls. No
// interleaving may expose a missing gate or an outside-workspace Allow.
func TestGateStaysClosedWhileReinitRacesRequests(t *testing.T) {
	c := &Controller{}
	c.initGate(permission.DefaultPolicy())
	req := permission.Request{
		Action: permission.ActionWrite,
		Tool:   "write",
		Paths:  []string{"/definitely/outside/f.txt"},
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 200 {
			c.initGate(permission.DefaultPolicy())
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			dec, _ := c.currentGate().Check(t.Context(), req)
			assert.Equal(t, permission.Deny, dec, "a re-init exposed an outside-workspace write")
		}
	}()
	wg.Wait()
}

// A controller whose gate was never assembled is not a permissive one.
func TestCurrentGateWithoutAssemblyDenies(t *testing.T) {
	dec, reason := (&Controller{}).currentGate().Check(t.Context(), permission.Request{
		Action: permission.ActionWrite,
		Paths:  []string{"/tmp/note.txt"},
	})

	assert.Equal(t, permission.Deny, dec)
	assert.Contains(t, reason, "never assembled")
}
