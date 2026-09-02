package permission

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// Gate evaluates permission requests. It has no side effects; Ask is handled by the caller.
type Gate interface {
	Check(ctx context.Context, req Request) (Decision, string)
}

// StaticGate evaluates against a fixed Policy and workspace root.
type StaticGate struct {
	Policy    Policy
	Workspace string

	bashAllow []*regexp.Regexp
	bashDeny  []*regexp.Regexp
	mcpAllow  []*regexp.Regexp
}

// NewGate compiles policy regexes and returns a Gate.
// Empty workspace uses WorkspaceRoot(). Workspace and memory dir are resolved
// to their physical targets so containment compares like with like (macOS
// resolves /tmp to /private/tmp); an unresolvable root fails construction
// closed instead of permitting everything.
func NewGate(policy Policy, workspace string) (*StaticGate, error) {
	if workspace == "" {
		workspace = WorkspaceRoot()
	}
	resolved, err := ResolveTarget(workspace)
	if err != nil {
		return nil, fmt.Errorf("permission gate: %w", err)
	}
	workspace = resolved
	if policy.MemoryDir != "" {
		resolved, err := ResolveTarget(policy.MemoryDir)
		if err != nil {
			return nil, fmt.Errorf("permission gate memory dir: %w", err)
		}
		policy.MemoryDir = resolved
	}
	// Sensitive prefixes are compared against resolved targets, so they are
	// resolved too: otherwise a prefix and a target on opposite sides of a
	// macOS-style /var -> /private/var symlink would never match. The resolved
	// list is a copy: policy arrives by value but its slice header does not, so
	// writing back would rewrite the caller's own deny list — and a caller that
	// builds one policy per gate would see the second gate resolve prefixes the
	// first one had already replaced.
	if len(policy.SensitivePathDeny) > 0 {
		resolvedDeny := make([]string, len(policy.SensitivePathDeny))
		for i, prefix := range policy.SensitivePathDeny {
			target, err := ResolveTarget(prefix)
			if err != nil {
				return nil, fmt.Errorf("permission gate sensitive path %q: %w", prefix, err)
			}
			resolvedDeny[i] = target
		}
		policy.SensitivePathDeny = resolvedDeny
	}
	g := &StaticGate{Policy: policy, Workspace: workspace}
	g.bashAllow, err = compilePatterns(policy.BashAllow)
	if err != nil {
		return nil, fmt.Errorf("bash allow: %w", err)
	}
	g.bashDeny, err = compilePatterns(policy.BashDeny)
	if err != nil {
		return nil, fmt.Errorf("bash deny: %w", err)
	}
	g.mcpAllow, err = compilePatterns(policy.MCPAllow)
	if err != nil {
		return nil, fmt.Errorf("mcp allow: %w", err)
	}
	return g, nil
}

func compilePatterns(patterns []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", p, err)
		}
		out = append(out, re)
	}
	return out, nil
}

// Check evaluates req and applies mode folding (Ask→Deny for headless-strict / autopilot).
func (g *StaticGate) Check(ctx context.Context, req Request) (Decision, string) {
	_ = ctx
	dec, reason := g.evaluate(req)
	return g.foldMode(dec, reason, req)
}

func (g *StaticGate) evaluate(req Request) (Decision, string) {
	switch req.Action {
	case ActionBash:
		return g.checkBash(req)
	case ActionWrite, ActionEdit:
		return g.checkWrite(req)
	case ActionRead, ActionGrep, ActionFind, ActionLs, ActionLSP:
		return g.checkRead(req)
	case ActionAgent:
		// Agent tools carry no paths the gate can vet: spawn confinement is
		// validated at job.Spawn against the parent workspace.
		return Allow, ""
	case ActionContext:
		// Quantitative usage report and own-context compaction only: the
		// transcript stays append-only, so there is nothing to gate.
		return Allow, ""
	case ActionMemory:
		return g.checkMemory()
	case ActionWatch:
		return g.checkWatch(req)
	case ActionTaskRead:
		return Allow, ""
	case ActionTaskWrite:
		// One note under the registry directory, named by a normalized id:
		// nothing outside it is reachable, so the write is allowed the way a
		// workspace write is. Readonly and plan modes still refuse it as the
		// mutation it is.
		return Allow, ""
	case ActionPlan:
		// Durable state belongs to this session and has no external
		// capability; hooks still observe and may deny the tool call.
		return Allow, ""
	case ActionQuestion:
		// The designated ask channel: an approval in front of the question
		// would only duplicate the prompt the user is about to answer anyway.
		return Allow, ""
	case ActionMCPList, ActionMCPInspect:
		// Read-only introspection of configured servers; no server code runs
		// and tool schemas stay off-context.
		return Allow, ""
	case ActionMCPCall:
		// A server tool is arbitrary capability the harness cannot see into,
		// so the default is to ask, naming the server and tool being handed
		// control. An explicit permissions.mcp.allow entry pre-approves it —
		// the only way a server works where no one can answer an ask (headless
		// runs, sub-agents), so the reason names that knob. A targetless
		// request never matches: pre-approval is by named server and tool.
		if req.Target != "" {
			for _, re := range g.mcpAllow {
				if re.MatchString(req.Target) {
					return Allow, ""
				}
			}
		}
		reason := "mcp_call requires approval"
		if req.Target != "" {
			reason += ": " + req.Target
		}
		reason += " (pre-approve via permissions.mcp.allow)"
		return Ask, reason
	default:
		return Ask, fmt.Sprintf("unknown action %q requires approval", req.Action)
	}
}

// checkMemory decides on the memory tool. It reads and archives inside the
// memory directory and nowhere else: the tool takes a name, never a path, and
// forgetting moves a file to forgotten/ rather than deleting it. A session
// without a memory directory has nothing for it to reach.
func (g *StaticGate) checkMemory() (Decision, string) {
	if g.Policy.MemoryDir == "" {
		return Deny, "no memory directory for this session"
	}
	return Allow, ""
}

// checkWatch decides on the watch tool. Starting one runs a shell command, so
// the bash deny list and the bash default both apply — nothing forbidden in
// bash becomes reachable by wrapping it in a watch.
//
// The bash allowlist deliberately does not apply. Those entries say a command
// is safe to run now, under a timeout, not safe to run forever: `^tail\b` is
// on the list, and `tail -f` as a watch never ends. So a watch asks even for
// an allowlisted command, and only an explicit allow-everything policy starts
// one unattended.
//
// The other actions (list, log, stop) carry no command and touch nothing.
func (g *StaticGate) checkWatch(req Request) (Decision, string) {
	cmd := strings.TrimSpace(req.Command)
	if cmd == "" {
		return Allow, ""
	}
	for _, re := range g.bashDeny {
		if re.MatchString(cmd) {
			return Deny, "watch denied by policy: matches " + re.String()
		}
	}
	switch g.Policy.BashDefault {
	case Allow:
		return Allow, ""
	case Deny:
		return Deny, "watch denied by default policy"
	default:
		return Ask, "watch requires approval — it runs in the background until stopped: " + truncate(cmd, 120)
	}
}

func (g *StaticGate) checkBash(req Request) (Decision, string) {
	cmd := strings.TrimSpace(req.Command)
	if cmd == "" {
		return Deny, "empty bash command denied"
	}
	for _, re := range g.bashDeny {
		if re.MatchString(cmd) {
			return Deny, "bash denied by policy: matches " + re.String()
		}
	}
	// Allowlist only applies to a single simple command. Prefix matches like
	// ^ls\b must not green-light "ls && rm -rf …".
	if bashEligibleForAllowlist(cmd) {
		for _, re := range g.bashAllow {
			if re.MatchString(cmd) {
				return Allow, ""
			}
		}
	}
	def := g.Policy.BashDefault
	if def == Allow {
		return Allow, ""
	}
	if def == Deny {
		return Deny, "bash denied by default policy"
	}
	return Ask, "bash requires approval: " + truncate(cmd, 120)
}

func (g *StaticGate) checkWrite(req Request) (Decision, string) {
	return g.checkPaths(req, g.Policy.WorkspaceOnlyWrites)
}

func (g *StaticGate) checkRead(req Request) (Decision, string) {
	return g.checkPaths(req, g.Policy.WorkspaceOnlyReads)
}

// mustNamePath reports whether an action is only meaningful with a path of its
// own: write and edit must name what they mutate, while the read family may
// default to the cwd. It is read off the action rather than passed in, so a
// caller cannot gate a write under the laxer rule by mistake.
func mustNamePath(action Action) bool {
	return action == ActionWrite || action == ActionEdit
}

// checkPaths judges every requested path by its physical filesystem target
// (see ResolveTarget): the gate compares like with like, so a leaf or ancestor
// symlink leading outside the workspace or into a sensitive path fails closed
// even when the displayed path looks contained.
func (g *StaticGate) checkPaths(req Request, workspaceOnly bool) (Decision, string) {
	if len(req.Paths) == 0 {
		if mustNamePath(req.Action) {
			return Deny, "write/edit without path denied"
		}
		// grep/find with default "." is normalized by extract; empty = allow cwd
		return Allow, ""
	}
	action := string(req.Action)
	for _, p := range req.Paths {
		resolved, err := ResolveTarget(p)
		if err != nil {
			return Deny, fmt.Sprintf("%s denied: cannot resolve %q: %v", action, p, err)
		}
		for _, q := range [2]string{p, resolved} {
			if IsSensitivePath(q, g.Policy.SensitivePathDeny) {
				return Deny, action + " of sensitive path denied: " + q
			}
		}
		// The memory-dir exemption holds only for the physical target: a
		// symlink planted in the memory dir must not smuggle an arbitrary
		// destination past the workspace rules.
		if !workspaceOnly || g.inMemoryDir(resolved) {
			continue
		}
		if !InWorkspace(resolved, g.Workspace) {
			if resolved == p {
				return Deny, action + " outside workspace denied: " + p
			}
			return Deny, fmt.Sprintf(
				"%s outside workspace denied: %s (%s resolves to %s)",
				action,
				resolved,
				p,
				resolved,
			)
		}
	}
	return Allow, ""
}

// inMemoryDir reports whether path is inside the agent's own memory directory.
// Checked after the sensitive-path deny, so the exemption can only lift the
// workspace rules.
func (g *StaticGate) inMemoryDir(path string) bool {
	return g.Policy.MemoryDir != "" && InWorkspace(path, g.Policy.MemoryDir)
}

func (g *StaticGate) foldMode(dec Decision, reason string, req Request) (Decision, string) {
	mode := g.Policy.Mode
	if mode == "" {
		mode = ModeInteractive
	}

	switch mode {
	case ModeInteractive:
		return dec, reason

	case ModeReadonly:
		if isMutating(req.Action) {
			// Allow bash only if it already matched allowlist (dec==Allow for bash).
			if req.Action == ActionBash && dec == Allow {
				return Allow, reason
			}
			if dec == Allow || dec == Ask {
				return Deny, readonlyReason(req, reason)
			}
		}
		if dec == Ask {
			return Deny, askFoldReason(reason, mode)
		}
		return dec, reason

	case ModeAutopilot, ModeHeadlessStrict:
		if dec == Ask {
			return Deny, askFoldReason(reason, mode)
		}
		return dec, reason

	default:
		return dec, reason
	}
}

func isMutating(a Action) bool {
	switch a {
	case ActionWrite, ActionEdit, ActionBash, ActionTaskWrite:
		return true
	default:
		return false
	}
}

func readonlyReason(req Request, fallback string) string {
	if fallback != "" && !strings.Contains(fallback, "requires approval") {
		return fallback
	}
	return fmt.Sprintf("readonly mode denies %s", req.Action)
}

func askFoldReason(reason string, mode Mode) string {
	if reason == "" {
		return fmt.Sprintf("%s mode denies operations that would require approval", mode)
	}
	return fmt.Sprintf("%s mode: %s", mode, reason)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// AllowAll is a Gate that always allows (tests / nil-policy fallback).
type AllowAll struct{}

// Check always returns Allow.
func (AllowAll) Check(context.Context, Request) (Decision, string) {
	return Allow, ""
}
