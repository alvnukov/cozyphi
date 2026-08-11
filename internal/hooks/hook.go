package hooks

import "context"

// Hook is the in-process extension point. Built-in policies, test fakes, and
// later compiled-in extensions implement this interface. External scripts are
// adapted to Hook by CommandHook (S7).
//
// Match reports whether this hook cares about tool (e.g. "bash"). The empty
// string or a Match that returns true for every tool means "all tools".
// When multiple hooks match the same event, call order is not guaranteed.
type Hook interface {
	Name() string
	Match(tool string) bool
	PreTool(ctx context.Context, ev Event) (PreResult, error)
	PostTool(ctx context.Context, ev Event) (PostResult, error)
}

// FuncHook implements Hook with closures. Useful in tests and thin wrappers.
// A nil MatchFn matches every tool. Nil Pre/Post return zero results.
type FuncHook struct {
	HookName string
	MatchFn  func(tool string) bool
	Pre      func(ctx context.Context, ev Event) (PreResult, error)
	Post     func(ctx context.Context, ev Event) (PostResult, error)
}

func (h FuncHook) Name() string {
	if h.HookName == "" {
		return "func"
	}
	return h.HookName
}

func (h FuncHook) Match(tool string) bool {
	if h.MatchFn == nil {
		return true
	}
	return h.MatchFn(tool)
}

func (h FuncHook) PreTool(ctx context.Context, ev Event) (PreResult, error) {
	if h.Pre == nil {
		return PreResult{Action: ActionAllow}, nil
	}
	return h.Pre(ctx, ev)
}

func (h FuncHook) PostTool(ctx context.Context, ev Event) (PostResult, error) {
	if h.Post == nil {
		return PostResult{}, nil
	}
	return h.Post(ctx, ev)
}

// MatchTool returns a MatchFn that equals a single tool name.
func MatchTool(name string) func(string) bool {
	return func(tool string) bool { return tool == name }
}

// MatchAll returns a MatchFn that accepts every tool.
func MatchAll() func(string) bool {
	return func(string) bool { return true }
}
