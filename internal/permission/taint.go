package permission

import "context"

// Taint is the turn's web-content state as the gate reads it: whether
// untrusted page text has already entered the model's context this turn, and
// which egress destinations the turn has already reached.
//
// It is an interface rather than a flag because the state belongs to the
// engine's turn, not to the boundary: the gate asks, it never records.
type Taint interface {
	// Tainted reports whether untrusted web text entered the context during
	// the current turn.
	Tainted() bool
	// HostSeen reports whether the turn already reached this egress host
	// with an approved call.
	HostSeen(host string) bool
}

// TaintGate re-asks for actions that would act on what a web page said.
//
// Once page text is in the context, every later decision in the turn was
// taken with the page's words in front of the model — including a decision
// the user pre-approved before the page was fetched. So the wrapper downgrades
// Allow to Ask for the actions a page would want: anything that mutates
// (bash, write, edit), anything that leaves the machine (mcp_call, a fetch or
// search to a host the turn has not used yet), and spawning an agent, which
// hands the same context to a fresh engine.
//
// It wraps the whole boundary, BypassGate included, because "allow all for
// this session" is a decision the user took before the page was read. Denials
// stand: a downgrade only ever makes a decision stricter.
//
// The wrapper adds no rules of its own: ModeOf and Observe see straight
// through it to the policy behind.
type TaintGate struct {
	Inner Gate
	Taint Taint
}

// Check defers to the inner gate and downgrades its Allow when the turn is
// tainted and the action is one a page would want.
func (g *TaintGate) Check(ctx context.Context, req Request) (Decision, string) {
	if g == nil || g.Inner == nil {
		return Deny, "permission gate unavailable: request denied (taint wrapper not assembled)"
	}
	dec, reason := g.Inner.Check(ctx, req)
	if dec != Allow || g.Taint == nil || !g.Taint.Tainted() {
		return dec, reason
	}
	if !downgradeAfterWeb(req, g.Taint) {
		return dec, reason
	}
	return Ask, taintReason(req)
}

// downgradeAfterWeb reports whether one action must be re-asked because
// untrusted web content already entered this turn.
func downgradeAfterWeb(req Request, taint Taint) bool {
	switch req.Action {
	case ActionWrite, ActionEdit, ActionBash, ActionMCPCall, ActionAgent:
		return true
	case ActionWeb:
		// Reading further from a document already in the cache is not a new
		// capability — the fetch that stored it was the decision. Reaching a
		// host this turn has not used is, and that is exactly the move a page
		// makes when it wants to be a beacon.
		return req.Host != "" && !taint.HostSeen(req.Host)
	default:
		return false
	}
}

// taintReason names both halves of the refusal: what is being asked for, and
// why an existing approval no longer covers it.
func taintReason(req Request) string {
	subject := Summarize(req)
	if subject == "" {
		subject = string(req.Action)
	}
	return string(req.Action) + " requires approval after web content in this turn: " + truncate(subject, 160)
}
