package permission

import "github.com/alvnukov/cozyphi/internal/tasks"

// Mode controls how Ask decisions are folded.
type Mode string

// Mode values control how Ask decisions are folded.
const (
	ModeInteractive    Mode = "interactive"
	ModeReadonly       Mode = "readonly"
	ModeAutopilot      Mode = "autopilot"
	ModeHeadlessStrict Mode = "headless-strict"
)

// Decision is the gate outcome before optional Ask folding.
type Decision int

// Decision values are the gate outcomes before optional Ask folding.
const (
	Allow Decision = iota
	Deny
	Ask
)

func (d Decision) String() string {
	switch d {
	case Allow:
		return "allow"
	case Deny:
		return "deny"
	case Ask:
		return "ask"
	default:
		return "unknown"
	}
}

// ModeOf returns the permission Mode configured on g, if known.
// BypassGate unwraps to its Inner. Unknown gate types return "".
func ModeOf(g Gate) Mode {
	for g != nil {
		switch x := g.(type) {
		case *StaticGate:
			if x.Policy.Mode != "" {
				return x.Policy.Mode
			}
			return ModeInteractive
		case *BypassGate:
			g = x.Inner
		case AllowAll:
			return ""
		default:
			return ""
		}
	}
	return ""
}

// Action names the kind of tool operation being gated.
type Action string

// Action values name the kinds of tool operations being gated.
const (
	ActionBash  Action = "bash"
	ActionRead  Action = "read"
	ActionWrite Action = "write"
	ActionEdit  Action = "edit"
	ActionGrep  Action = "grep"
	ActionFind  Action = "find"
	ActionLs    Action = "ls"
	ActionAgent Action = "agent"
	// ActionLSP covers the read-only lsp tool; it maps to read policy.
	ActionLSP Action = "lsp"
	// ActionContext covers the context tool: quantitative usage report and
	// own-context compaction. No filesystem, network or subprocess effects.
	ActionContext Action = "context"
	// ActionPlan covers durable plan read/update within the current session.
	// It carries no filesystem, network, or subprocess capability.
	ActionPlan Action = "plan"

	// ActionMemory covers the memory tool. It carries no path the gate could
	// vet — a memory is addressed by name inside the session's own memory
	// directory — so what the gate decides is whether that directory exists at
	// all, which is what tells a sub-agent apart from the session the user is
	// sitting in.
	ActionMemory Action = "memory"

	// ActionWatch covers starting a background watch. Its command is a shell
	// command, so the bash deny list and default apply — but it outlives the
	// tool call that started it, and a polling watch re-runs on every tick, so
	// one approval buys every later run. The bash allowlist does not carry
	// over: those entries clear a command to run once under a timeout, not to
	// run forever. Listing, reading and stopping watches need no approval.
	ActionWatch Action = "watch"

	// ActionTaskRead and ActionTaskWrite cover the task tool. The registry is
	// a fixed directory of the main checkout, found at startup and addressed
	// by normalized ids, so there is no path for the gate to vet. What
	// decides is Policy.Tasks, the user's own setting: a note is
	// bookkeeping about the work, not the work, so the level applies the
	// same in every mode — plan mode included, where shaping tasks is the
	// point.
	ActionTaskRead  Action = "task_read"
	ActionTaskWrite Action = "task_write"

	// ActionQuestion covers the question tool: the designated channel for the
	// model to ask the user. It renders a prompt and returns the user's
	// choice, so allowing it costs nothing an approval would protect — an Ask
	// in front of it would only duplicate the prompt it already is.
	ActionQuestion Action = "question"

	// ActionMCPList and ActionMCPInspect cover the read-only MCP meta-tools:
	// tool names and one tool's parameter summary. No server code runs and
	// schemas stay off-context either way.
	ActionMCPList    Action = "mcp_list"
	ActionMCPInspect Action = "mcp_inspect"

	// ActionMCPCall covers mcp_call: it hands control to a configured server's
	// tool — arbitrary capability the harness cannot see into — so it asks,
	// naming the server and tool being handed control.
	ActionMCPCall Action = "mcp_call"
)

// Request describes a tool invocation for permission evaluation.
type Request struct {
	Action  Action
	Tool    string
	Paths   []string // absolute, cleaned
	Command string
	Target  string // named capability being approved, e.g. server/tool for mcp_call

	// Preview is display-only evidence for the ask overlay — the diff an
	// edit or write would apply. The executor attaches it after the gate
	// has decided to ask; it never takes part in policy evaluation.
	Preview string
}

// Policy is the configurable permission ruleset.
type Policy struct {
	Mode                Mode
	WorkspaceOnlyWrites bool
	AskTimeoutSec       int
	BashDefault         Decision // typically Ask
	BashAllow           []string // regex
	BashDeny            []string // regex
	SensitivePathDeny   []string // path prefixes
	WorkspaceOnlyReads  bool     // if true, out-of-workspace reads deny
	DangerouslyAllowAll bool     // skip all permission checks

	// MCPAllow pre-approves mcp_call targets as regexes matched against
	// "server/tool" (e.g. `^github/`). Every server tool is arbitrary
	// capability the harness cannot see into, so the default is to ask — but
	// an ask nobody can answer turns every call into a denial in headless
	// runs and sub-agents. The list is the explicit opt-in that keeps a
	// configured server usable there.
	MCPAllow []string // regex, matched against server/tool

	// Tasks is permissions.tasks: how far the model may go with the task
	// registry (off, read, ask, write). Empty reads as write, the default,
	// so a Policy built by hand behaves like a configured one.
	Tasks tasks.Access

	// MemoryDir is the Claude Code auto-memory directory shared by both agents.
	// It lives outside the workspace (~/.claude/projects/<project>/memory), so
	// without this
	// the workspace-only rules would deny the agent the facts the harness
	// told it to keep. The exemption lifts only those rules — a sensitive
	// path stays denied — and an empty value disables it, which is what a
	// sub-agent gets.
	MemoryDir string
}

// DefaultPolicy returns the interactive defaults from task-002.
func DefaultPolicy() Policy {
	return Policy{
		Mode:                ModeInteractive,
		WorkspaceOnlyWrites: true,
		AskTimeoutSec:       120,
		Tasks:               tasks.AccessWrite,
		BashDefault:         Ask,
		BashAllow:           defaultBashAllow,
		BashDeny:            defaultBashDeny,
		SensitivePathDeny:   defaultSensitivePaths(),
		WorkspaceOnlyReads:  false,
	}
}
