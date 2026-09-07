package permission

import (
	"slices"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// Reasons a boundary's rules cannot be read. They are written here rather
// than at the call site because each one is the whole explanation a user
// gets for a permissions view that reports nothing: it has to say what stands
// there instead and what that thing does to a request.
const (
	reasonUnknownGate = "the installed boundary is not one this view can read; " +
		"its rules stay unreported rather than being guessed from the configuration"
	reasonAllowAllGate = "no rules at all: every request is allowed without being judged"
	reasonNilBypass    = "the bypass wrapper is nil; it denies every request"
	reasonNoInnerGate  = "the bypass wrapper has no boundary behind it; it denies every request"
	reasonNilStatic    = "the compiled boundary is nil; it has no rules to read"
)

// PolicyObservation projects one policy into what the harness may say about
// it. It is an allowlist by construction: the members of diag.PermissionFacts
// exist because they are safe to publish, and a bash pattern, a sensitive
// path prefix, an mcp allow entry, a web egress pattern and the memory
// directory have no member here to land in — each is reduced to a count or to
// a presence before it leaves this function, so no rule literal can reach the
// view.
//
// An unset mode is reported as interactive because that is how Check folds
// it; an unset task level as write, for the same reason. Reporting the empty
// string would describe a session nobody is running.
func PolicyObservation(p Policy) diag.PermissionFacts {
	mode := p.Mode
	if mode == "" {
		mode = ModeInteractive
	}
	return diag.PermissionFacts{
		Known:               true,
		Mode:                string(mode),
		BashDefault:         p.BashDefault.String(),
		BashAllow:           len(p.BashAllow),
		BashDeny:            len(p.BashDeny),
		BashAllowIsDefault:  slices.Equal(p.BashAllow, defaultBashAllow),
		BashDenyIsDefault:   slices.Equal(p.BashDeny, defaultBashDeny),
		SensitivePaths:      len(p.SensitivePathDeny),
		MCPAllow:            len(p.MCPAllow),
		WebAllow:            len(p.WebAllow),
		WorkspaceOnlyWrites: p.WorkspaceOnlyWrites,
		WorkspaceOnlyReads:  p.WorkspaceOnlyReads,
		AskTimeoutSec:       p.AskTimeoutSec,
		Tasks:               string(p.Tasks.Normalized()),
		Memory:              p.MemoryDir != "",
		AllowAll:            p.DangerouslyAllowAll,
	}
}

// Observe projects the assembled boundary — the gate together with every
// wrapper around it — into what the harness may say about it.
//
// It reads the boundary and never asks it to decide anything: no Request is
// built here, no Check is called, no command runs, no path is resolved and no
// approval is simulated. Observing therefore grants nothing and changes no
// later decision.
//
// A boundary this function does not recognize is reported as unknown, with a
// reason, and its policy stays unknown. Unwrapping to the configured policy
// or to the defaults would describe rules that are not the ones deciding,
// which is the one answer a permissions view must never give.
func Observe(g Gate) diag.GateFacts {
	if g == nil {
		return diag.GateFacts{}
	}
	facts := diag.GateFacts{Known: true}
	for {
		switch x := g.(type) {
		case *BypassGate:
			if x == nil {
				return unreadable(facts, reasonNilBypass)
			}
			// Recorded before descending: whether the switch is on is a fact
			// about the wrapper, and it decides every request before the
			// boundary behind it is consulted at all.
			facts.Bypassable = true
			facts.Bypassing = x.Enabled != nil && x.Enabled.Load()
			if x.Inner == nil {
				return unreadable(facts, reasonNoInnerGate)
			}
			g = x.Inner
		case *TaintGate:
			// The turn-taint wrapper carries no rules of its own: it only
			// makes an Allow stricter while a page's words sit in the
			// context. Reporting it as a layer would describe a policy that
			// is not there, so the view reads straight through it.
			if x == nil || x.Inner == nil {
				return unreadable(facts, reasonNoInnerGate)
			}
			g = x.Inner
		case *StaticGate:
			if x == nil {
				return unreadable(facts, reasonNilStatic)
			}
			facts.Kind = diag.GateStatic
			facts.Policy = PolicyObservation(x.Policy)
			return facts
		case AllowAll:
			// Not a wrapper but a whole boundary with nothing inside it, so
			// it is reported as bypassing rather than as a policy whose every
			// rule happens to permit.
			facts.Kind = diag.GateAllowAll
			facts.Bypassable = true
			facts.Bypassing = true
			facts.Reason = reasonAllowAllGate
			return facts
		case UnavailableGate:
			facts.Kind = diag.GateUnavailable
			facts.Reason = x.Reason
			return facts
		default:
			facts.Kind = diag.GateUnknown
			facts.Reason = reasonUnknownGate
			return facts
		}
	}
}

// unreadable finishes an observation of a half-assembled boundary. There is
// no policy behind it to read, so it is reported as unavailable with what
// failed. A bypass recorded on the way down is left standing: BypassGate
// answers its switch before it looks for an inner gate, so a wrapper that is
// switched on allows every request even with nothing behind it, and saying
// otherwise would understate what the session can do.
func unreadable(facts diag.GateFacts, reason string) diag.GateFacts {
	facts.Kind = diag.GateUnavailable
	facts.Reason = reason
	return facts
}

// DefaultObservation is the built-in policy as the harness sees it: what a
// session with no permissions block at all runs on. Comparing an observed
// layer against it is how a view tells a rule a config file set from one
// nobody has ever touched, without asking the loader to remember which keys
// the file spelled.
func DefaultObservation() diag.PermissionFacts {
	return PolicyObservation(DefaultPolicy())
}
