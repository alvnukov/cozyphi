package permission

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/tasks"
)

// The sentinels below are the rule text a user writes: a command pattern, a
// path prefix, an mcp target, an egress destination and the memory directory.
// Every one of them can name something private — the deny list of a real
// machine says what is on it, and an egress allow entry names a host on a
// network nobody else can see — so none of them may appear in an observation.
const (
	allowSentinel     = `^deploy --to prod-cluster-sentinel\b`
	denySentinel      = `\bcurl .*internal-vault-sentinel\b`
	sensitiveSentinel = "sensitive-dir-sentinel"
	mcpSentinel       = "^vault-server-sentinel/"
	webSentinel       = `^wiki\.internal-sentinel\.example$`
	memorySentinel    = "memory-dir-sentinel"
)

// sentinelPolicy is a fully configured policy whose every list carries a
// value nobody may repeat.
func sentinelPolicy(t *testing.T) Policy {
	t.Helper()
	root := t.TempDir()
	return Policy{
		Mode:                ModeInteractive,
		WorkspaceOnlyWrites: true,
		WorkspaceOnlyReads:  true,
		AskTimeoutSec:       90,
		BashDefault:         Ask,
		BashAllow:           []string{allowSentinel},
		BashDeny:            []string{denySentinel, `\bsudo\b`},
		SensitivePathDeny:   []string{filepath.Join(root, sensitiveSentinel)},
		MCPAllow:            []string{mcpSentinel},
		WebAllow:            []string{webSentinel},
		Tasks:               tasks.AccessAsk,
		MemoryDir:           filepath.Join(root, memorySentinel),
	}
}

// encoded renders facts the way the harness would, so an assertion about what
// leaves is made against everything that leaves rather than field by field.
func encoded(t *testing.T, facts diag.GateFacts) string {
	t.Helper()
	out, err := json.Marshal(facts)
	require.NoError(t, err)
	return string(out)
}

// countingGate stands in for anything that decorates the boundary. It records
// every request it is handed, so a test can prove an observation handed it
// none.
type countingGate struct {
	inner  Gate
	checks atomic.Int64
}

func (g *countingGate) Check(ctx context.Context, req Request) (Decision, string) {
	g.checks.Add(1)
	if g.inner == nil {
		return Deny, "no inner gate"
	}
	return g.inner.Check(ctx, req)
}

func TestObservingAStaticBoundaryCountsItsRulesAndQuotesNone(t *testing.T) {
	policy := sentinelPolicy(t)
	gate, err := NewGate(policy, t.TempDir())
	require.NoError(t, err)

	facts := Observe(gate)

	require.True(t, facts.Known)
	assert.Equal(t, diag.GateStatic, facts.Kind)
	assert.False(t, facts.Bypassable, "a bare compiled boundary has nothing in front of it")
	require.True(t, facts.Policy.Known)
	assert.Equal(t, "interactive", facts.Policy.Mode)
	assert.Equal(t, "ask", facts.Policy.BashDefault)
	assert.Equal(t, 1, facts.Policy.BashAllow)
	assert.Equal(t, 2, facts.Policy.BashDeny)
	assert.Equal(t, 1, facts.Policy.SensitivePaths)
	assert.Equal(t, 1, facts.Policy.MCPAllow)
	assert.Equal(t, 1, facts.Policy.WebAllow,
		"an egress destination is a count like every other rule: it names a host, and a host is a fact "+
			"about the user's network rather than about this session")
	assert.True(t, facts.Policy.WorkspaceOnlyWrites)
	assert.True(t, facts.Policy.WorkspaceOnlyReads)
	assert.Equal(t, 90, facts.Policy.AskTimeoutSec)
	assert.Equal(t, "ask", facts.Policy.Tasks)
	assert.True(t, facts.Policy.Memory, "a bound memory directory is a presence, never a path")

	rendered := encoded(t, facts)
	for _, secret := range []string{
		allowSentinel, denySentinel, sensitiveSentinel, mcpSentinel, webSentinel, memorySentinel,
		"sudo", "sentinel",
	} {
		assert.NotContains(t, rendered, secret, "a rule is counted, never quoted")
	}
}

func TestABoundaryNamesWhoseRulesItIsRunning(t *testing.T) {
	defaults, err := NewGate(DefaultPolicy(), t.TempDir())
	require.NoError(t, err)
	replaced, err := NewGate(sentinelPolicy(t), t.TempDir())
	require.NoError(t, err)

	builtin := Observe(defaults).Policy
	assert.True(t, builtin.BashAllowIsDefault, "an untouched allowlist is the built-in one")
	assert.True(t, builtin.BashDenyIsDefault)
	assert.Equal(t, len(defaultBashAllow), builtin.BashAllow)
	assert.Equal(t, len(defaultBashDeny), builtin.BashDeny)

	custom := Observe(replaced).Policy
	assert.False(t, custom.BashAllowIsDefault, "a replaced allowlist is the configuration's")
	assert.False(t, custom.BashDenyIsDefault)
}

func TestAnUnsetModeIsReportedAsTheOneCheckActuallyFolds(t *testing.T) {
	gate, err := NewGate(Policy{}, t.TempDir())
	require.NoError(t, err)

	policy := Observe(gate).Policy

	assert.Equal(t, string(ModeInteractive), policy.Mode,
		"Check treats an unset mode as interactive, so the view says interactive")
	assert.Equal(t, string(tasks.AccessWrite), policy.Tasks,
		"an unset task level reads as write, the same way the gate reads it")
}

func TestABypassWrapperIsReportedWithItsSwitchAndTheRulesBehindIt(t *testing.T) {
	inner, err := NewGate(sentinelPolicy(t), t.TempDir())
	require.NoError(t, err)
	var enabled atomic.Bool
	gate := &BypassGate{Inner: inner, Enabled: &enabled}

	off := Observe(gate)
	assert.Equal(t, diag.GateStatic, off.Kind, "the boundary behind the wrapper is still what decides")
	assert.True(t, off.Bypassable)
	assert.False(t, off.Bypassing)
	assert.True(t, off.Policy.Known, "a switched-off wrapper hides none of the rules")

	enabled.Store(true)
	on := Observe(gate)
	assert.True(t, on.Bypassing, "the switch is the whole difference")
	assert.True(t, on.Policy.Known, "the rules are still there; they are just not being consulted")
}

func TestAWrapperWithNothingBehindItStillReportsWhatItAllows(t *testing.T) {
	var enabled atomic.Bool
	enabled.Store(true)

	// BypassGate answers its switch before it looks for an inner gate, so
	// this half-assembled wrapper allows every request. Reporting it as a
	// denial would understate what the session can do.
	open := Observe(&BypassGate{Enabled: &enabled})
	assert.Equal(t, diag.GateUnavailable, open.Kind)
	assert.True(t, open.Bypassing)
	assert.NotEmpty(t, open.Reason)
	assert.False(t, open.Policy.Known)

	enabled.Store(false)
	closed := Observe(&BypassGate{Enabled: &enabled})
	assert.Equal(t, diag.GateUnavailable, closed.Kind)
	assert.False(t, closed.Bypassing, "with the switch off the same wrapper denies everything")
}

func TestAnUnassembledBoundaryCarriesWhatFailed(t *testing.T) {
	facts := Observe(UnavailableGate{Reason: "bash allow: invalid pattern"})

	require.True(t, facts.Known)
	assert.Equal(t, diag.GateUnavailable, facts.Kind)
	assert.Equal(t, "bash allow: invalid pattern", facts.Reason)
	assert.False(t, facts.Policy.Known, "a boundary that never compiled has no rules to report")
	assert.False(t, facts.Bypassing)
}

func TestABoundaryWithNoRulesSaysSoRatherThanLookingPermissive(t *testing.T) {
	facts := Observe(AllowAll{})

	assert.Equal(t, diag.GateAllowAll, facts.Kind)
	assert.True(t, facts.Bypassing, "nothing is judged, so nothing is being applied")
	assert.NotEmpty(t, facts.Reason)
	assert.False(t, facts.Policy.Known,
		"there is no policy here; an empty one would read as rules that permit")
}

func TestABoundaryNobodyRecognizesIsUnknownRatherThanPermissive(t *testing.T) {
	inner, err := NewGate(sentinelPolicy(t), t.TempDir())
	require.NoError(t, err)
	decorated := &countingGate{inner: inner}

	facts := Observe(decorated)

	require.True(t, facts.Known)
	assert.Equal(t, diag.GateUnknown, facts.Kind)
	assert.NotEmpty(t, facts.Reason, "an unreadable boundary says why it is unreadable")
	assert.False(t, facts.Policy.Known,
		"the rules behind a decorator are not the decorator's, so none are reported")
	assert.False(t, facts.Bypassing, "unknown is not allow-all")
	assert.Zero(t, decorated.checks.Load(), "observing never asks a boundary to decide anything")
	assert.NotContains(t, encoded(t, facts), "sentinel")
}

func TestObservingNeverHandsTheBoundaryARequest(t *testing.T) {
	inner, err := NewGate(DefaultPolicy(), t.TempDir())
	require.NoError(t, err)
	spy := &countingGate{inner: inner}
	var enabled atomic.Bool

	Observe(&BypassGate{Inner: spy, Enabled: &enabled})
	enabled.Store(true)
	Observe(&BypassGate{Inner: spy, Enabled: &enabled})

	assert.Zero(t, spy.checks.Load(),
		"a permission view reads the boundary; it never probes it with a request")
}

func TestANilBoundaryObservesNothingRatherThanPanicking(t *testing.T) {
	assert.Equal(t, diag.GateFacts{}, Observe(nil))
	assert.Equal(t, diag.GateUnavailable, Observe((*BypassGate)(nil)).Kind)
	assert.Equal(t, diag.GateUnavailable, Observe((*StaticGate)(nil)).Kind)
}

func TestObservingChangesNothingTheBoundaryWouldDecide(t *testing.T) {
	root := t.TempDir()
	gate, err := NewGate(sentinelPolicy(t), root)
	require.NoError(t, err)
	requests := []Request{
		{Action: ActionBash, Command: "ls -la"},
		{Action: ActionBash, Command: "sudo rm -rf /"},
		{Action: ActionWrite, Paths: []string{filepath.Join(root, "a.go")}},
		{Action: ActionRead, Paths: []string{filepath.Join(t.TempDir(), "b.go")}},
		{Action: ActionMCPCall, Target: "github/create_issue"},
		{Action: ActionTaskWrite, Target: "some-task"},
	}

	before := decisions(t, gate, requests)
	facts := Observe(gate)
	require.True(t, facts.Policy.Known)
	after := decisions(t, gate, requests)

	assert.Equal(t, before, after, "reading the policy grants nothing and withdraws nothing")
}

func decisions(t *testing.T, gate Gate, requests []Request) []string {
	t.Helper()
	out := make([]string, 0, len(requests))
	for _, req := range requests {
		decision, reason := gate.Check(t.Context(), req)
		out = append(out, decision.String()+": "+reason)
	}
	return out
}
