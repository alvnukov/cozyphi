package tooldef

import "encoding/json"

// PlanStep is the plan gate's plan_step argument as tools receive it. Tools
// never read the value — the executor consumes it before dispatch — but
// strict decoding must accept a gate-valid call in every input shape: the v2
// stable step id string and the legacy 1-based number.
type PlanStep struct{}

// UnmarshalJSON accepts any value; the field exists only to reserve the name.
func (*PlanStep) UnmarshalJSON([]byte) error { return nil }

var _ json.Unmarshaler = (*PlanStep)(nil)
