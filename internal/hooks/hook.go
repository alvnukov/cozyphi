package hooks

import "context"

// Hook is the in-process extension point. Built-in policies, test fakes, and
// later compiled-in extensions implement this interface. External scripts are
// adapted to Hook by CommandHook.
//
// Match reports whether this hook cares about tool (e.g. "bash"). The empty
// string or a Match that returns true for every tool means "all tools".
// When multiple hooks match the same event, call order is not guaranteed.
// Command is only invoked for KindCommand entries.
type Hook interface {
	Name() string
	Match(tool string) bool
	PreTool(ctx context.Context, ev Event) (PreResult, error)
	PostTool(ctx context.Context, ev Event) (PostResult, error)
	Command(ctx context.Context, ev CommandEvent) (CommandResult, error)
}

// FuncHook implements Hook with closures. Useful in tests and thin wrappers.
// A nil MatchFn matches every tool. Nil Pre/Post return zero results.
type FuncHook struct {
	HookName string
	MatchFn  func(tool string) bool
	Pre      func(ctx context.Context, ev Event) (PreResult, error)
	Post     func(ctx context.Context, ev Event) (PostResult, error)
	Cmd      func(ctx context.Context, ev CommandEvent) (CommandResult, error)
}

// Name returns the hook name, defaulting to "func" when unset.
func (h FuncHook) Name() string {
	if h.HookName == "" {
		return "func"
	}
	return h.HookName
}

// Match reports whether this hook cares about the tool; a nil MatchFn matches all tools.
func (h FuncHook) Match(tool string) bool {
	if h.MatchFn == nil {
		return true
	}
	return h.MatchFn(tool)
}

// PreTool invokes the Pre closure, allowing the tool when Pre is nil.
func (h FuncHook) PreTool(ctx context.Context, ev Event) (PreResult, error) {
	if h.Pre == nil {
		return PreResult{Action: ActionAllow}, nil
	}
	return h.Pre(ctx, ev)
}

// PostTool invokes the Post closure, returning an empty result when Post is nil.
func (h FuncHook) PostTool(ctx context.Context, ev Event) (PostResult, error) {
	if h.Post == nil {
		return PostResult{}, nil
	}
	return h.Post(ctx, ev)
}

// Command invokes the Cmd closure, returning an empty result when Cmd is nil.
func (h FuncHook) Command(ctx context.Context, ev CommandEvent) (CommandResult, error) {
	if h.Cmd == nil {
		return CommandResult{}, nil
	}
	return h.Cmd(ctx, ev)
}

// MatchTool returns a MatchFn that equals a single tool name.
func MatchTool(name string) func(string) bool {
	return func(tool string) bool { return tool == name }
}

// MatchAll returns a MatchFn that accepts every tool.
func MatchAll() func(string) bool {
	return func(string) bool { return true }
}
