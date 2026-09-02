package permission

import "context"

// UnavailableGate denies every request. It stands in wherever a boundary could
// not be assembled — a policy the gate cannot compile, a workspace root that
// will not resolve — so a session with no working rules refuses tool calls
// instead of running them unchecked. Reason names what failed, so the refusal
// tells the user what to fix rather than reading as a mysterious denial.
type UnavailableGate struct {
	Reason string
}

// Check always denies, naming the assembly failure.
func (g UnavailableGate) Check(context.Context, Request) (Decision, string) {
	reason := g.Reason
	if reason == "" {
		reason = "the policy could not be compiled"
	}
	return Deny, "permission gate failed to assemble: " + reason
}
