package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
)

func testEngine(t *testing.T, opts EngineOpts) *Engine {
	t.Helper()
	opts.Model = llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"}
	if opts.SessionOpts.Cwd == "" {
		opts.SessionOpts.Cwd = t.TempDir()
	}
	eng, err := NewEngine(opts)
	require.NoError(t, err)
	return eng
}

// TestEveryEngineGateIsWrappedForTaint: the downgrade sits outside whatever
// gate the session runs, so a bypass gate ("allow all this session") cannot
// route around it.
func TestEveryEngineGateIsWrappedForTaint(t *testing.T) {
	eng := testEngine(t, EngineOpts{})
	wrapper, ok := eng.executor.gate.(*permission.TaintGate)
	require.True(t, ok, "the executor gate must be the taint wrapper")
	assert.NotNil(t, wrapper.Inner, "a nil inner gate would make the wrapper an allow-all")
	assert.NotNil(t, wrapper.Taint)
}

// TestWebTextTaintsTheTurnAndLoopClearsIt: the mark lives for one turn — long
// enough to cover the decisions taken with the page in view, short enough not
// to nag through the rest of the session.
func TestWebTextTaintsTheTurnAndLoopClearsIt(t *testing.T) {
	eng := testEngine(t, EngineOpts{})
	eng.turnWeb.mark()
	require.True(t, eng.turnWeb.Tainted())

	dec, reason := eng.executor.gate.Check(t.Context(),
		permission.Request{Action: permission.ActionBash, Command: "curl attacker.example"})
	assert.Equal(t, permission.Ask, dec)
	assert.Contains(t, reason, "after web content in this turn")

	// Draining the turn is enough: the reset is the first thing Loop does,
	// well before the (unreachable) model is dialed.
	for range eng.Loop(t.Context(), "hello", LoopOpts{}) { //nolint:revive // the error is the point
	}
	assert.False(t, eng.turnWeb.Tainted(), "a new turn must start clean")
}

// TestApprovedHostsAreRememberedForTheTurn: re-asking for the host the user
// just approved would train them to click through the one question that
// matters.
func TestApprovedHostsAreRememberedForTheTurn(t *testing.T) {
	eng := testEngine(t, EngineOpts{})
	eng.turnWeb.mark()

	fetch := permission.Request{
		Action: permission.ActionWeb, Op: "fetch", Host: "docs.example.com", Target: "https://docs.example.com/x",
	}
	dec, _ := eng.executor.gate.Check(t.Context(), fetch)
	require.Equal(t, permission.Ask, dec, "a fresh host after web content must be asked about")

	eng.observeWebApproval(fetch)
	dec, _ = eng.executor.gate.Check(t.Context(), fetch)
	assert.Equal(t, permission.Allow, dec)

	other := fetch
	other.Host = "attacker.example"
	dec, _ = eng.executor.gate.Check(t.Context(), other)
	assert.Equal(t, permission.Ask, dec, "the grant covers one host, not the network")

	eng.turnWeb.reset()
	assert.False(t, eng.turnWeb.HostSeen("docs.example.com"), "host grants die with the turn")
}

// TestNonWebApprovalsDoNotGrantHosts guards the observer's narrow job.
func TestNonWebApprovalsDoNotGrantHosts(t *testing.T) {
	eng := testEngine(t, EngineOpts{})
	eng.observeWebApproval(permission.Request{Action: permission.ActionBash, Host: "docs.example.com"})
	assert.False(t, eng.turnWeb.HostSeen("docs.example.com"))
}

// TestWebToolFollowsTheConfiguration: no `web:` section, no tool — the model
// cannot be asked to use a capability the user never turned on.
func TestWebToolFollowsTheConfiguration(t *testing.T) {
	assert.False(t, testEngine(t, EngineOpts{}).HasTool("web"))
	assert.True(t, testEngine(t, EngineOpts{Web: testWebOptions(t)}).HasTool("web"))
}
