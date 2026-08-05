package permission

// Mode controls how Ask decisions are folded.
type Mode string

const (
	ModeInteractive    Mode = "interactive"
	ModeReadonly       Mode = "readonly"
	ModeAutopilot      Mode = "autopilot"
	ModeHeadlessStrict Mode = "headless-strict"
)

// Decision is the gate outcome before optional Ask folding.
type Decision int

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

// Action names the kind of tool operation being gated.
type Action string

const (
	ActionBash  Action = "bash"
	ActionRead  Action = "read"
	ActionWrite Action = "write"
	ActionEdit  Action = "edit"
	ActionFetch Action = "fetch"
	ActionGrep  Action = "grep"
	ActionGlob  Action = "glob"
	ActionList  Action = "list"
)

// Request describes a tool invocation for permission evaluation.
type Request struct {
	Action  Action
	Tool    string
	Paths   []string // absolute, cleaned
	Command string
	URL     string
}

// Policy is the configurable permission ruleset.
type Policy struct {
	Mode                 Mode
	WorkspaceOnlyWrites  bool
	AskTimeoutSec        int
	BashDefault          Decision // typically Ask
	BashAllow            []string // regex
	BashDeny             []string // regex
	FetchDefault         Decision // typically Ask
	FetchAllowedHosts    []string
	SensitivePathDeny    []string // path prefixes
	WorkspaceOnlyReads   bool     // if true, out-of-workspace reads deny
	DangerouslyAllowAll  bool     // skip all permission checks
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
		FetchDefault:        Ask,
		FetchAllowedHosts:   nil,
		SensitivePathDeny:   defaultSensitivePaths(),
		WorkspaceOnlyReads:  false,
	}
}
