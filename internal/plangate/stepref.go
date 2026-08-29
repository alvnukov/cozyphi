package plangate

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/alvnukov/cozyphi/internal/session"
)

// legacyStepNote rides a passing call that named a step by number instead of
// its stable id: numeric input keeps working until the migration retires it,
// and every answer says so.
const legacyStepNote = "plan_step numbers are deprecated: pass the step's stable id; call plan with action get to list ids."

// StepRef names the plan step a tool call advances. The v2 form is the step's
// stable id; Ordinal carries the legacy 1-based number some models still send.
// Exactly one of the two is set by StepFromArgs, which is the only constructor
// callers need.
type StepRef struct {
	ID      string
	Ordinal int
}

// String renders the reference for miss reasons: the id, the number, or a
// placeholder for an omitted reference.
func (r StepRef) String() string {
	switch {
	case r.ID != "":
		return strconv.Quote(r.ID)
	case r.Ordinal > 0:
		return strconv.Itoa(r.Ordinal)
	default:
		return "(omitted)"
	}
}

// Find resolves the reference against a plan: a stable id matches Items[].ID,
// a legacy ordinal is a positional 1-based index. ok is false when the
// reference is absent or matches no step.
func (r StepRef) Find(plan session.Plan) (session.PlanItem, bool) {
	if r.ID != "" {
		for _, item := range plan.Items {
			if item.ID == r.ID {
				return item, true
			}
		}
		return session.PlanItem{}, false
	}
	if r.Ordinal >= 1 && r.Ordinal <= len(plan.Items) {
		return plan.Items[r.Ordinal-1], true
	}
	return session.PlanItem{}, false
}

// StepFromArgs reads the plan_step argument out of raw tool arguments. A JSON
// string is a stable step id, a JSON number is the legacy 1-based ordinal, and
// anything else — absent, null, wrong type, undecodable — is an omitted
// reference the gate answers as a miss.
func StepFromArgs(args json.RawMessage) StepRef {
	var in struct {
		PlanStep json.RawMessage `json:"plan_step"`
	}
	if json.Unmarshal(args, &in) != nil {
		return StepRef{}
	}
	raw := strings.TrimSpace(string(in.PlanStep))
	if raw == "" || raw == "null" {
		return StepRef{}
	}
	var id string
	if json.Unmarshal([]byte(raw), &id) == nil {
		return StepRef{ID: strings.TrimSpace(id)}
	}
	var ordinal int
	if json.Unmarshal([]byte(raw), &ordinal) == nil {
		return StepRef{Ordinal: ordinal}
	}
	return StepRef{}
}
