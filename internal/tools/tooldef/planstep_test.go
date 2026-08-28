package tooldef

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// The contract: a strict-decoding tool that declares PlanStep accepts every
// plan_step input shape the gate lets through.
func TestPlanStepAcceptsEveryGateValidShape(t *testing.T) {
	for _, raw := range []string{
		`{"plan_step":"wire-schema"}`,
		`{"plan_step":2}`,
		`{"plan_step":null}`,
		`{"plan_step":true}`,
		`{}`,
	} {
		var in struct {
			PlanStep PlanStep `json:"plan_step"`
		}
		require.NoError(t, DecodeStrict(json.RawMessage(raw), &in), raw)
	}
}
