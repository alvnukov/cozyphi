package plangate

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

func TestSplitEnvelopeAbsentLeavesArgumentsUntouched(t *testing.T) {
	raw := json.RawMessage(`{"path":"a.go","plan_step":"wire"}`)
	envelope, cleaned, err := SplitEnvelope(raw)
	require.NoError(t, err)
	assert.True(t, envelope.Empty())
	assert.JSONEq(t, string(raw), string(cleaned))

	// Non-object arguments carry no envelope; they pass through untouched
	// for the tool's own decoding to reject.
	envelope, cleaned, err = SplitEnvelope(json.RawMessage(`[]`))
	require.NoError(t, err)
	assert.True(t, envelope.Empty())
	assert.Equal(t, "[]", string(cleaned))
}

func TestSplitEnvelopeStripsAndDecodes(t *testing.T) {
	raw := json.RawMessage(
		`{"path":"a.go","plan_step":"wire","_plan":{"complete":{"stepId":"prev","outcome":"done","evidence":"e"},"workingContext":"fresh"}}`,
	)
	envelope, cleaned, err := SplitEnvelope(raw)
	require.NoError(t, err)

	require.NotNil(t, envelope.Complete)
	assert.Equal(t, session.TransitionComplete, envelope.Complete.Action,
		"the envelope normalizes the complete action")
	assert.Equal(t, "prev", envelope.Complete.StepID)
	require.NotNil(t, envelope.WorkingContext)
	assert.Equal(t, "fresh", *envelope.WorkingContext)
	assert.JSONEq(t, `{"path":"a.go","plan_step":"wire"}`, string(cleaned))
}

func TestSplitEnvelopeRejectsMalformed(t *testing.T) {
	cases := map[string]string{
		"unknown field":  `{"_plan":{"bogus":1}}`,
		"empty envelope": `{"_plan":{}}`,
		"foreign action": `{"_plan":{"complete":{"stepId":"p","action":"block","blocker":"b","resumeWhen":"r"}}}`,
		"not an object":  `{"_plan":"settle"}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := SplitEnvelope(json.RawMessage(raw))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "_plan")
		})
	}
}

func TestSettleMutationIDDeterministicAndSlugSafe(t *testing.T) {
	first := SettleMutationID("call_abc")
	assert.Equal(t, first, SettleMutationID("call_abc"), "retries derive the same key")
	assert.NotEqual(t, first, SettleMutationID("call_abd"))
	assert.Regexp(t, `^piggyback-[0-9a-f]{16}$`, first)
}
