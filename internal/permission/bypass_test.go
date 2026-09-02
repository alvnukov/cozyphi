package permission

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func denyWorthyWrite() Request {
	return Request{Action: ActionWrite, Tool: "write", Paths: []string{"/definitely/outside/f.txt"}}
}

// An incomplete assembly must deny: a nil gate or a missing inner used to
// Allow, silently removing the permission boundary (fail-open).
func TestBypassGateIncompleteAssemblyDenies(t *testing.T) {
	var enabled atomic.Bool
	req := denyWorthyWrite()

	var nilGate *BypassGate
	dec, reason := nilGate.Check(t.Context(), req)
	assert.Equal(t, Deny, dec, "a nil gate must deny")
	assert.NotEmpty(t, reason, "the denial must say what is wrong")

	dec, reason = (&BypassGate{Enabled: &enabled}).Check(t.Context(), req)
	assert.Equal(t, Deny, dec, "a missing inner gate must deny")
	assert.NotEmpty(t, reason)
}

// Only an explicitly enabled bypass may return unconditional Allow.
func TestBypassGateEnabledAllowsWithoutInner(t *testing.T) {
	var enabled atomic.Bool
	enabled.Store(true)

	dec, _ := (&BypassGate{Enabled: &enabled}).Check(t.Context(), denyWorthyWrite())
	assert.Equal(t, Allow, dec, "an explicit bypass stays unconditional")

	var nilGate *BypassGate
	dec, _ = nilGate.Check(t.Context(), denyWorthyWrite())
	assert.Equal(t, Deny, dec, "even so, a nil gate cannot speak for a policy")
}

func TestBypassGateDefersToInnerWhenDisabled(t *testing.T) {
	inner, err := NewGate(Policy{Mode: ModeReadonly}, t.TempDir())
	require.NoError(t, err)
	var enabled atomic.Bool
	bypass := &BypassGate{Inner: inner, Enabled: &enabled}

	dec, _ := bypass.Check(t.Context(), denyWorthyWrite())
	assert.Equal(t, Deny, dec, "disabled bypass defers to the inner gate")
}
