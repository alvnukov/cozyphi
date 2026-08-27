package permission

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
)

// Request describes a tool invocation for permission evaluation.
type Request struct {
	Action  Action
	Tool    string
	Paths   []string // absolute, cleaned
	Command string
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

	// MemoryDir is the agent's own memory directory. It lives outside the
	// workspace by design (~/.cozyphi/memory/<encoded-cwd>/), so without this
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
		BashDefault:         Ask,
		BashAllow:           defaultBashAllow,
		BashDeny:            defaultBashDeny,
		SensitivePathDeny:   defaultSensitivePaths(),
		WorkspaceOnlyReads:  false,
	}
}
