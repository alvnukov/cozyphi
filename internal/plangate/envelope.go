package plangate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

// envelopeKey is the harness-owned argument every strict tool schema omits:
// the model may attach it to a working tool call, and the executor consumes
// it before the call reaches hooks, the permission gate or the tool itself.
const envelopeKey = "_plan"

// Envelope is the plan metadata one working call piggybacks: complete the
// previous step and swap the plan's working context in the same round that
// runs the next tool. Both halves are optional; an envelope that settles
// nothing is an error, not a silent no-op.
type Envelope struct {
	Complete       *session.PlanTransition `json:"complete,omitempty"`
	WorkingContext *string                 `json:"workingContext,omitempty"`
}

// Empty reports whether the envelope carries no work.
func (e Envelope) Empty() bool {
	return e.Complete == nil && e.WorkingContext == nil
}

// SplitEnvelope separates the harness-owned _plan envelope from a tool
// call's own arguments. The cleaned arguments keep every other key and drop
// only _plan, so strict tool decoding never sees harness metadata; a
// malformed envelope is an error the caller must reject the whole call with,
// because the model clearly meant to settle something and silently dropping
// it would desynchronize the plan from the transcript. Arguments that are
// not a JSON object carry no envelope and pass through untouched for the
// tool to reject.
func SplitEnvelope(raw json.RawMessage) (Envelope, json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Envelope{}, raw, nil
	}
	value, ok := fields[envelopeKey]
	if !ok {
		return Envelope{}, raw, nil
	}
	var envelope Envelope
	if err := tooldef.DecodeStrict(value, &envelope); err != nil {
		return Envelope{}, nil, fmt.Errorf("plangate: %s must be a plan envelope object: %w", envelopeKey, err)
	}
	if envelope.Empty() {
		return Envelope{}, nil, fmt.Errorf("plangate: %s must carry complete or workingContext", envelopeKey)
	}
	if envelope.Complete != nil {
		switch envelope.Complete.Action {
		case "", session.TransitionComplete:
			envelope.Complete.Action = session.TransitionComplete
		default:
			return Envelope{}, nil, fmt.Errorf(
				"plangate: %s may only complete a step, not %q", envelopeKey, envelope.Complete.Action,
			)
		}
	}
	delete(fields, envelopeKey)
	cleaned, err := json.Marshal(fields)
	if err != nil {
		return Envelope{}, nil, fmt.Errorf("plangate: rebuild arguments without %s: %w", envelopeKey, err)
	}
	return envelope, cleaned, nil
}

// SettleMutationID derives the settle's idempotency key from the tool call
// id: deterministic across retries and reconciliation, a valid plan mutation
// slug for any provider's call id shape, and collision-safe through the hash
// tail.
func SettleMutationID(callID string) string {
	sum := sha256.Sum256([]byte("plan-settle:" + callID))
	return "piggyback-" + hex.EncodeToString(sum[:8])
}
