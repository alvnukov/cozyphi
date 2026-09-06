package permission

import "github.com/alvnukov/cozyphi/internal/diag"

// toolChecks says, for each tool this package judges, what the boundary has
// to read in order to decide one call. It is a description of Extract and
// StaticGate.evaluate above, written down so a view can explain a tool
// without evaluating anything: nothing here builds a Request, resolves a
// path, matches a pattern or consults a policy.
//
// The classification is deliberately coarse and deliberately pessimistic. A
// tool whose decision reads its own arguments is never described as allowed,
// because the answer belongs to a call nobody has written yet — a bash
// command matches the deny list or it does not, a path is inside the
// workspace or it is not, an mcp_call target is pre-approved or it is not.
// Saying "allowed" from the name alone is exactly the promise this view must
// not make.
var toolChecks = map[string]diag.ToolCheck{
	// The command is matched against the deny list, the allow list and the
	// default, in that order.
	"bash": diag.CheckArguments,
	// The path decides: read containment, the sensitive-path list, and for
	// writes the workspace boundary and the memory directory.
	"read":  diag.CheckArguments,
	"write": diag.CheckArguments,
	"edit":  diag.CheckArguments,
	"grep":  diag.CheckArguments,
	"find":  diag.CheckArguments,
	"ls":    diag.CheckArguments,
	"lsp":   diag.CheckArguments,
	// Starting a watch runs a shell command, so the bash rules judge it; the
	// other operations address a watch by id and carry nothing to judge.
	"watch": diag.CheckArguments,
	// The server and tool named in the call are matched against
	// permissions.mcp.allow; without a match the call needs an approval.
	"mcp_call": diag.CheckArguments,
	// The action argument decides whether the call reads the registry or
	// changes a note; permissions.tasks then decides how far it may go.
	"task": diag.CheckArguments,
	// The memory tool takes a name, never a path: whether a memory directory
	// is bound to this session is the whole decision, and it is the same for
	// every call.
	"memory": diag.CheckPolicy,
	// Decided by the tool alone. Naming the session always asks; the rest
	// carry no path, no command and no external capability, so the boundary
	// answers the same way every time.
	"session":      diag.CheckToolName,
	"plan":         diag.CheckToolName,
	"question":     diag.CheckToolName,
	"context":      diag.CheckToolName,
	"harness":      diag.CheckToolName,
	"mcp_list":     diag.CheckToolName,
	"mcp_inspect":  diag.CheckToolName,
	"agent_spawn":  diag.CheckToolName,
	"agent_list":   diag.CheckToolName,
	"agent_wait":   diag.CheckToolName,
	"agent_cancel": diag.CheckToolName,
}

// ToolCheckKind reports what the boundary reads to decide one tool's call.
// It answers from the table above and touches no gate: observing what a
// decision would cost must never be a decision.
//
// A tool this package does not judge by name — a caller-supplied one, say —
// is reported unknown rather than assumed harmless. The default branch of
// Extract hands such a call to the gate under its own name, and the gate's
// default is to ask, so guessing on its behalf here would be a guess in the
// permissive direction.
func ToolCheckKind(name string) diag.ToolCheck {
	if kind, ok := toolChecks[name]; ok {
		return kind
	}
	return diag.CheckUnknown
}
