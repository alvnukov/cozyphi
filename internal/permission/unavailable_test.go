package permission

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The stand-in for a boundary that could not be built denies everything, and
// says what failed so the refusal is actionable.
func TestUnavailableGateDeniesAndNamesTheFailure(t *testing.T) {
	gate := UnavailableGate{Reason: "resolve target \"\": empty path"}

	for _, req := range []Request{
		{Action: ActionBash, Command: "ls"},
		{Action: ActionWrite, Paths: []string{"/tmp/note.txt"}},
		{Action: ActionRead, Paths: []string{"/tmp/note.txt"}},
		{Action: ActionQuestion},
	} {
		dec, reason := gate.Check(t.Context(), req)
		require.Equal(t, Deny, dec, "action %s must be denied", req.Action)
		assert.Contains(t, reason, "permission gate failed to assemble")
		assert.Contains(t, reason, "empty path")
	}
}

// A reasonless failure still denies with a sentence a user can act on.
func TestUnavailableGateWithoutReasonStillExplains(t *testing.T) {
	dec, reason := UnavailableGate{}.Check(t.Context(), Request{Action: ActionWrite})

	assert.Equal(t, Deny, dec)
	assert.Contains(t, reason, "permission gate failed to assemble")
	assert.NotEmpty(t, reason)
}
