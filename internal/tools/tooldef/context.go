package tooldef

import (
	"context"
	"strings"
)

type toolCallIDKey struct{}

type cwdKey struct{}

// WithToolCallID attaches the active tool_use id to ctx for UI correlation.
func WithToolCallID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, toolCallIDKey{}, id)
}

// ToolCallID returns the tool_use id from ctx, or empty.
func ToolCallID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(toolCallIDKey{}).(string)
	return v
}

// WithCwd attaches the session working directory used to resolve relative tool paths.
// Empty cwd is ignored so callers can pass through an unset session cwd.
func WithCwd(ctx context.Context, cwd string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return ctx
	}
	return context.WithValue(ctx, cwdKey{}, cwd)
}

func cwdFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(cwdKey{}).(string)
	return strings.TrimSpace(v)
}

type mutationGuardKey struct{}

// MutationGuard re-applies the permission rules to a path at the moment a tool
// is about to change it. The gate resolves and judges a path when the call is
// approved; a local racing process can swap a directory into the path between
// that verdict and the mutation, so the module performing the swap calls the
// guard again with the same path and refuses the write on a non-nil error.
type MutationGuard func(ctx context.Context, path string) error

// WithMutationGuard attaches guard to ctx. A nil guard is ignored so callers
// can pass through an unconfigured gate.
func WithMutationGuard(ctx context.Context, guard MutationGuard) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if guard == nil {
		return ctx
	}
	return context.WithValue(ctx, mutationGuardKey{}, guard)
}

// GuardMutation runs the guard from ctx against path. Without a guard the
// mutation proceeds: the seam is a re-check of a verdict that has already been
// made, never the only place a path is judged.
func GuardMutation(ctx context.Context, path string) error {
	if ctx == nil {
		return nil
	}
	guard, _ := ctx.Value(mutationGuardKey{}).(MutationGuard)
	if guard == nil {
		return nil
	}
	return guard(ctx, path)
}
